package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	aesGCMNonceSize = 12
	aesGCMTagSize   = 16 // GCM 认证标签长度
)

// chunkSize 控制每个明文分块的大小。
// 每块达到此大小时立即加密并写入底层 writer，避免缓冲全部数据。
const chunkSize = 64 * 1024 // 64KB

// AESGCMEncryptor 使用 AES-GCM 认证加密算法进行流式加密和解密。
// 内部采用分块加密模式：数据达到 chunkSize（64KB）时立即加密写入，
// 避免大数据量时的内存暴涨。
//
// 密钥长度决定 AES 变体：16 字节 = AES-128，24 字节 = AES-192，32 字节 = AES-256。
type AESGCMEncryptor struct {
	key []byte
}

// NewAESGCMEncryptor 创建基于 AES-GCM 的加密器。
// key: AES 密钥，合法长度为 16、24 或 32 字节。
func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	switch len(key) {
	case 16, 24, 32:
		return &AESGCMEncryptor{key: key}, nil
	default:
		return nil, errors.Newf("invalid AES key length: %d (must be 16, 24, or 32)", len(key))
	}
}

// AlgoID 返回 AES-GCM 加密算法的标识字节。
func (e *AESGCMEncryptor) AlgoID() byte { return AlgoEncryptionAESGCM }

// WrapWriter 包装 io.Writer，返回一个写入即加密的 WriteCloser。
// 采用分块加密模式，每块格式：[4B 密文长度][12B nonce][密文]。
// 长度前缀使解密端能逐块读取，无需全部缓冲。
func (e *AESGCMEncryptor) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	return &aesGCMWriteCloser{
		w:         w,
		key:       e.key,
		chunkSize: chunkSize,
	}, nil
}

// WrapReader 包装 io.Reader，返回一个读取即解密的 ReadCloser。
// 输入格式为块密文：[4B 密文长度][12B nonce][密文]...
func (e *AESGCMEncryptor) WrapReader(r io.Reader) (io.ReadCloser, error) {
	return &aesGCMReadCloser{
		r:   r,
		key: e.key,
	}, nil
}

// ---------------------------------------------------------------------------
// aesGCMWriteCloser — 分块加密写入器
// ---------------------------------------------------------------------------

type aesGCMWriteCloser struct {
	w         io.Writer
	key       []byte
	chunkSize int
	buf       bytes.Buffer // 当前未满块的明文缓冲区
	closed    bool
	mu        sync.Mutex
}

func (wc *aesGCMWriteCloser) Write(p []byte) (int, error) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	if wc.closed {
		return 0, errors.New("aes-gcm writer is already closed")
	}

	n := len(p)
	written := 0
	remaining := p

	for len(remaining) > 0 {
		// 填充缓冲区到 chunkSize
		space := wc.chunkSize - wc.buf.Len()
		if space <= 0 {
			// 缓冲区已满，加密并刷新
			if err := wc.flushLocked(); err != nil {
				return written, errors.WithStack(err)
			}
			space = wc.chunkSize
		}

		// 取 min(space, len(remaining)) 字节写入缓冲区
		take := space
		if take > len(remaining) {
			take = len(remaining)
		}
		wc.buf.Write(remaining[:take])
		remaining = remaining[take:]
		written += take
	}

	return n, nil
}

func (wc *aesGCMWriteCloser) Close() error {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	if wc.closed {
		return nil
	}
	wc.closed = true

	// flush 剩余不满一块的数据（可能为 0，即空输入）
	return wc.flushLocked()
}

// flushLocked 加密并写入缓冲区中的全部数据（调用者持有 mu）。
func (wc *aesGCMWriteCloser) flushLocked() error {
	if wc.buf.Len() == 0 {
		return nil
	}

	plaintext := wc.buf.Bytes()
	wc.buf.Reset()

	return wc.encryptAndWrite(plaintext)
}

func (wc *aesGCMWriteCloser) encryptAndWrite(plaintext []byte) error {
	// 生成随机 nonce
	nonce := make([]byte, aesGCMNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return errors.WithStack(err)
	}

	block, err := aes.NewCipher(wc.key)
	if err != nil {
		return errors.WithStack(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return errors.WithStack(err)
	}

	// 加密
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// 写入 [4B 密文长度][12B nonce][密文]
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))

	if _, err := wc.w.Write(lenBuf[:]); err != nil {
		return errors.WithStack(err)
	}
	if _, err := wc.w.Write(nonce); err != nil {
		return errors.WithStack(err)
	}
	if _, err := wc.w.Write(ciphertext); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// aesGCMReadCloser — 分块解密读取器
// ---------------------------------------------------------------------------

type aesGCMReadCloser struct {
	r          io.Reader
	key        []byte
	buf        bytes.Buffer // 已解密待读取的明文
	exhausted  bool         // 所有块已读取完毕
	initOnce   sync.Once
	initErr    error
	closed     bool
	mu         sync.Mutex
}

func (rc *aesGCMReadCloser) Read(p []byte) (int, error) {
	// 延迟初始化
	rc.initOnce.Do(func() {
		rc.initErr = rc.initGCM()
	})
	if rc.initErr != nil {
		return 0, rc.initErr
	}

	// 当缓冲区无数据时，尝试读取下一个块
	for rc.buf.Len() == 0 && !rc.exhausted {
		if err := rc.readNextChunk(); err != nil {
			if err == io.EOF {
				rc.exhausted = true
				break
			}
			return 0, errors.WithStack(err)
		}
	}

	return rc.buf.Read(p)
}

func (rc *aesGCMReadCloser) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.closed = true
	return nil
}

func (rc *aesGCMReadCloser) initGCM() error {
	return nil
}

func (rc *aesGCMReadCloser) readNextChunk() error {
	// 读取 4B 密文长度
	var lenBuf [4]byte
	if _, err := io.ReadFull(rc.r, lenBuf[:]); err != nil {
		return err // 可能返回 io.EOF
	}
	cipherLen := binary.BigEndian.Uint32(lenBuf[:])

	// 读取 nonce
	nonce := make([]byte, aesGCMNonceSize)
	if _, err := io.ReadFull(rc.r, nonce); err != nil {
		return errors.WithStack(err)
	}

	// 读取密文
	ciphertext := make([]byte, cipherLen)
	if _, err := io.ReadFull(rc.r, ciphertext); err != nil {
		return errors.WithStack(err)
	}

	// 解密
	block, err := aes.NewCipher(rc.key)
	if err != nil {
		return errors.WithStack(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return errors.WithStack(err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	rc.buf.Write(plaintext)
	return nil
}

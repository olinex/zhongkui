package combine

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"

	"zhongkuisec.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// 加密算法标识常量
const (
	ModeEncryptionAesGcm byte = 0x01
)

const (
	// ChunkSize 控制每个明文分块的大小。
	// 每块达到此大小时立即加密并写入底层 writer，避免缓冲全部数据。
	ChunkSize = 64 * 1024 // 64KB
)

// 编译期断言：确保 AESGCMCryptor 实现了 Transform 和 ModeIDer 接口
var (
	_ Transform = (*AESGCMCryptor)(nil)
	_ ModeIDer  = (*AESGCMCryptor)(nil)
)

// AESGCMCryptor 使用 AES-GCM 认证加密算法进行流式加密和解密。
// 内部采用分块加密模式：数据达到 chunkSize（64KB）时立即加密写入，
// 避免大数据量时的内存暴涨。
//
// 密钥长度决定 AES 变体：16 字节 = AES-128，24 字节 = AES-192，32 字节 = AES-256。
type AESGCMCryptor struct {
	// key AES 密钥，长度决定 AES 变体（16/24/32 字节）。
	key []byte
}

// NewAESGCMCryptor 创建基于 AES-GCM 的加密器。
// key: AES 密钥，合法长度为 16、24 或 32 字节。
func NewAESGCMCryptor(cryptoKey []byte) (*AESGCMCryptor, error) {
	switch len(cryptoKey) {
	case 16, 24, 32:
		return &AESGCMCryptor{key: cryptoKey}, nil
	default:
		return nil, errors.Newf("invalid AES key length: %d (must be 16, 24, or 32)", len(cryptoKey))
	}
}

// ModeID 返回 AES-GCM 加密算法的标识字节。
func (e *AESGCMCryptor) ModeID() byte { return ModeEncryptionAesGcm }

// WrapWriter 包装 io.Writer，返回一个写入即加密的 WriteCloser。
// 采用分块加密模式，每块格式：[4B 密文长度][12B nonce][密文]。
// 长度前缀使解密端能逐块读取，无需全部缓冲。
func (e *AESGCMCryptor) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	return &aesGCMWriteCloser{
		w:         w,
		key:       e.key,
		chunkSize: ChunkSize,
	}, nil
}

// WrapReader 包装 io.Reader，返回一个读取即解密的 ReadCloser。
// 输入格式为块密文：[4B 密文长度][12B nonce][密文]...
func (e *AESGCMCryptor) WrapReader(r io.Reader) (io.ReadCloser, error) {
	return &aesGCMReadCloser{
		r:   r,
		key: e.key,
	}, nil
}

// ---------------------------------------------------------------------------
// aesGCMWriteCloser — 分块加密写入器
// ---------------------------------------------------------------------------

type aesGCMWriteCloser struct {
	mu        sync.Mutex
	w         io.Writer
	key       []byte
	chunkSize int
	buf       bytes.Buffer // 当前未满块的明文缓冲区
	closed    bool
}

// Write 将 p 写入内部缓冲区，缓冲满一块（chunkSize）时立即加密并写入底层 writer，返回已接收字节数。
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
		take := min(space, len(remaining))
		wc.buf.Write(remaining[:take])
		remaining = remaining[take:]
		written += take
	}

	return n, nil
}

// Close 加密并写入缓冲区中剩余的不完整块后关闭写入器；重复关闭安全幂等。
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
	mode, err := mode.NewAESGCMMode(wc.key, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	// 加密
	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		return errors.WithStack(err)
	}
	// 写入 [4B 密文长度][12B nonce][密文]
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ciphertext)))
	if _, err := wc.w.Write(lenBuf[:]); err != nil {
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
	mu        sync.Mutex
	r         io.Reader
	key       []byte
	buf       bytes.Buffer // 已解密待读取的明文
	exhausted bool         // 所有块已读取完毕
	closed    bool
}

// Read 从已解密的缓冲区读取明文，缓冲区为空时解密下一个密文块，读到流末尾返回 io.EOF。
func (rc *aesGCMReadCloser) Read(p []byte) (int, error) {
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

// Close 关闭读取器；重复关闭安全幂等。
func (rc *aesGCMReadCloser) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.closed = true
	return nil
}

func (rc *aesGCMReadCloser) readNextChunk() error {
	// 读取 4B 密文长度
	var lenBuf [4]byte
	if _, err := io.ReadFull(rc.r, lenBuf[:]); err != nil {
		return err // 可能返回 io.EOF
	}
	cipherLen := binary.BigEndian.Uint32(lenBuf[:])
	// 读取密文
	ciphertext := make([]byte, cipherLen)
	if _, err := io.ReadFull(rc.r, ciphertext); err != nil {
		return errors.WithStack(err)
	}
	// 解密
	mode, err := mode.NewAESGCMMode(rc.key, nil)
	if err != nil {
		return errors.WithStack(err)

	}
	plaintext, err := mode.Decrypt(ciphertext)
	if err != nil {
		return errors.WithStack(err)
	}
	rc.buf.Write(plaintext)
	return nil
}

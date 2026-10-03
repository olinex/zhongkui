package network

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/s3"
)

// mockS3Server 内存版 S3 mock 服务器
// 支持单次 PUT / HEAD / GET / DELETE，记录每个对象键的上传次数，用于断言去重逻辑
type mockS3Server struct {
	mu      sync.Mutex
	objects map[string][]byte // 对象键 -> 上传的密文内容
	puts    map[string]int    // 对象键 -> PUT 上传次数
}

func newMockS3Server() *mockS3Server {
	return &mockS3Server{
		objects: make(map[string][]byte),
		puts:    make(map[string]int),
	}
}

// ServeHTTP 实现内存版 S3 服务端，按方法处理 PUT/HEAD/GET/DELETE 请求。
func (m *mockS3Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
	m.mu.Lock()
	defer m.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		data, err := readPutBody(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		m.objects[key] = data
		m.puts[key]++
		m.writeObjectHeaders(w, data)
		w.WriteHeader(http.StatusOK)
	case http.MethodHead:
		if data, ok := m.objects[key]; ok {
			m.writeObjectHeaders(w, data)
			w.WriteHeader(http.StatusOK)
			return
		}
		m.writeNoSuchKey(w)
	case http.MethodGet:
		if data, ok := m.objects[key]; ok {
			m.writeObjectHeaders(w, data)
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
		m.writeNoSuchKey(w)
	case http.MethodDelete:
		delete(m.objects, key)
		delete(m.puts, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// writeObjectHeaders 写入 StatObject/GetObject 解析所需的响应头
func (m *mockS3Server) writeObjectHeaders(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, md5.Sum(data)))
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
}

// writeNoSuchKey 返回 NoSuchKey 错误，使 IsNotExists 能识别对象不存在
func (m *mockS3Server) writeNoSuchKey(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
}

// readPutBody 读取上传请求体。
// minio-go 对 V4 签名的上传默认使用 aws-chunked 流式分块（X-Amz-Content-Sha256 为
// STREAMING-AWS4-HMAC-SHA256-PAYLOAD），Go 的 net/http 不会透明解码这种应用层分块，
// 需要按 aws-chunked 帧格式（<hex-size>;chunk-signature=<sig>\r\n<data>\r\n ... 0;...\r\n\r\n）手动解码。
func readPutBody(r *http.Request) ([]byte, error) {
	if !strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
		return io.ReadAll(r.Body)
	}
	br := bufio.NewReader(r.Body)
	var buf bytes.Buffer
	for {
		header, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.ParseInt(strings.SplitN(strings.TrimRight(header, "\r\n"), ";", 2)[0], 16, 64)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			break
		}
		if _, err := io.CopyN(&buf, br, size); err != nil {
			return nil, err
		}
		if _, err := br.Discard(2); err != nil { // 消费 chunk 数据后的 CRLF
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// TestNewHttpURIFileExt 验证 NewHttpURI 正确填充 FileExt 字段
// 扩展名提取的具体逻辑由 utils.FileExtension 负责，此处仅验证字段接线
func TestNewHttpURIFileExt(t *testing.T) {
	Convey("Test NewHttpURI file extension field", t, func() {
		Convey("文件资源请求应提取文件扩展名", func() {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/static/js/app.js", nil)
			uri, err := NewHttpURI(req)
			So(err, ShouldBeNil)
			So(uri.FileExt, ShouldEqual, "js")
		})

		Convey("无扩展名的 API 请求 FileExt 应为空", func() {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/api/users", nil)
			uri, err := NewHttpURI(req)
			So(err, ShouldBeNil)
			So(uri.FileExt, ShouldEqual, "")
		})
	})
}

// TestStoreBodyDedup 验证 storeBody 的内容寻址去重逻辑
// 相同大 Body 应只上传一次并复用同一对象键；不同内容生成不同对象键
func TestStoreBodyDedup(t *testing.T) {
	Convey("Test storeBody content-addressed dedup", t, func() {
		provider := key.NewConfigFileSecretKeyProvider("0123456789abcdef0123456789abcdef")
		book, err := key.LoadPasswordBook("")
		So(err, ShouldBeNil)
		kd := key.NewKeyDerivation(provider, book)
		// 全局密钥派生器供 crypto.HashKeyForField 使用（摘要密钥来源与生产一致）
		key.InitGlobalKeyDerivation(provider, book)

		mock := newMockS3Server()
		server := httptest.NewServer(mock)
		defer server.Close()

		cfg := &s3.Config{
			Endpoint:        strings.TrimPrefix(server.URL, "http://"),
			AccessKey:       "test-access-key",
			SecretKey:       "test-secret-key",
			Bucket:          "test-bucket",
			Region:          "us-east-1",
			UseSSL:          false,
			StreamingUpload: false, // 默认路径：FPutObject 单次 PUT
		}
		client, err := s3.NewS3Client(cfg, kd)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		ctx := context.Background()
		cryptoKey := []byte("0123456789abcdef0123456789abcdef")
		// 摘要在生产由字段 secure 标签派生密钥，测试沿用同一条链路
		reqHashKey, err := crypto.HashKeyForField(&HttpRequestBody{}, "reqBodyHash", "json")
		So(err, ShouldBeNil)
		rspHashKey, err := crypto.HashKeyForField(&HttpResponseBody{}, "rspBodyHash", "json")
		So(err, ShouldBeNil)

		Convey("相同大 Body 只上传一次，返回相同对象键", func() {
			// 约 1.75MB 的大 Body，重复两次
			body := bytes.Repeat([]byte("dedup-payload-"), 128*1024)
			res1, err := storeBody(ctx, client, bytes.NewReader(body), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)
			res2, err := storeBody(ctx, client, bytes.NewReader(body), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)

			So(res1.Key, ShouldNotBeEmpty)
			So(res2.Key, ShouldEqual, res1.Key)
			// 摘要为带密钥 HMAC-SHA256，与长度一并如实记录
			expectedMac := hmac.New(sha256.New, reqHashKey)
			expectedMac.Write(body)
			So(res1.Hash, ShouldEqual, hex.EncodeToString(expectedMac.Sum(nil)))
			So(res1.Length, ShouldEqual, int64(len(body)))

			mock.mu.Lock()
			So(mock.puts[res1.Key], ShouldEqual, 1) // 第二次未触发上传
			So(len(mock.objects), ShouldEqual, 1)
			mock.mu.Unlock()

			// 回读去重后的对象，验证内容完整且可解密
			rc, err := s3.NewZkcn(client, res1.Key, cryptoKey, 0).Load(ctx)
			So(err, ShouldBeNil)
			defer rc.Close()
			data, err := io.ReadAll(rc)
			So(err, ShouldBeNil)
			So(data, ShouldResemble, body)
		})

		Convey("不同内容生成不同对象键", func() {
			resA, err := storeBody(ctx, client, bytes.NewReader(bytes.Repeat([]byte("A"), 1<<20)), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)
			resB, err := storeBody(ctx, client, bytes.NewReader(bytes.Repeat([]byte("B"), 1<<20)), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)
			So(resA.Key, ShouldNotEqual, resB.Key)
			So(resA.Key, ShouldNotBeEmpty)
			So(resB.Key, ShouldNotBeEmpty)
			So(resA.Hash, ShouldNotEqual, resB.Hash)

			mock.mu.Lock()
			So(len(mock.objects), ShouldEqual, 2)
			mock.mu.Unlock()
		})

		Convey("请求与响应方向使用不同前缀，不互相去重", func() {
			body := bytes.Repeat([]byte("X"), 1<<20)
			resReq, err := storeBody(ctx, client, bytes.NewReader(body), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)
			resRsp, err := storeBody(ctx, client, bytes.NewReader(body), false, cryptoKey, rspHashKey)
			So(err, ShouldBeNil)
			So(resReq.Key, ShouldNotEqual, resRsp.Key)
			So(strings.HasPrefix(resReq.Key, "body/request/"), ShouldBeTrue)
			So(strings.HasPrefix(resRsp.Key, "body/response/"), ShouldBeTrue)
			// 两个方向的摘要密钥不同（offset 12808 / 12809），同一内容摘要亦不同
			So(resReq.Hash, ShouldNotEqual, resRsp.Hash)
			So(resReq.Length, ShouldEqual, resRsp.Length)

			mock.mu.Lock()
			So(len(mock.objects), ShouldEqual, 2)
			mock.mu.Unlock()
		})

		Convey("空内容不执行上传", func() {
			res, err := storeBody(ctx, client, strings.NewReader(""), true, cryptoKey, reqHashKey)
			So(err, ShouldBeNil)
			So(res.Key, ShouldEqual, "")
			// 空内容记录 0 长度与空内容的摘要
			So(res.Length, ShouldEqual, 0)
			emptyMac := hmac.New(sha256.New, reqHashKey)
			So(res.Hash, ShouldEqual, hex.EncodeToString(emptyMac.Sum(nil)))

			mock.mu.Lock()
			So(len(mock.objects), ShouldEqual, 0)
			mock.mu.Unlock()
		})
	})
}

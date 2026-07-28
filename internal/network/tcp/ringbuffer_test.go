package tcp

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smallnest/ringbuffer"
)

// =============================================================================
// bufio.Reader 与 RingBuffer 基础交互测试
// =============================================================================

// TestBufioReader_Read_Basic 验证 bufio.Reader 从 RingBuffer 基础读取
func TestBufioReader_Read_Basic(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	payload := []byte("hello world")
	if _, err := rb.Write(payload); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(rb)
	buf := make([]byte, len(payload))
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if n != len(payload) {
		t.Fatalf("expected %d bytes, got %d", len(payload), n)
	}
	if string(buf) != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", string(buf))
	}
}

// TestBufioReader_Read_MultipleWrites 验证多次写入 RingBuffer 后 bufio.Reader 分次读取
func TestBufioReader_Read_MultipleWrites(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("first "))
	rb.Write([]byte("second "))
	rb.Write([]byte("third"))

	reader := bufio.NewReader(rb)

	// 分三次读取
	for _, expected := range []string{"first ", "second ", "third"} {
		buf := make([]byte, len(expected))
		n, err := io.ReadFull(reader, buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(expected) || string(buf) != expected {
			t.Fatalf("expected '%s', got '%s'", expected, string(buf))
		}
	}

	// RingBuffer 应该已被读空
	if rb.Length() != 0 {
		t.Fatalf("ringbuffer should be empty, got %d bytes", rb.Length())
	}
}

// TestBufioReader_ReadLine 验证 bufio.Reader.ReadLine / ReadString 等高级方法
func TestBufioReader_ReadLine(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("line1\nline2\nline3\n"))

	reader := bufio.NewReader(rb)

	for i, expected := range []string{"line1", "line2", "line3"} {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\n")
		if line != expected {
			t.Fatalf("line %d: expected '%s', got '%s'", i, expected, line)
		}
	}
}

// =============================================================================
// bufio.Writer 与 RingBuffer 基础交互测试
// =============================================================================

// TestBufioWriter_Write_Basic 验证 bufio.Writer 写入 RingBuffer
func TestBufioWriter_Write_Basic(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	writer := bufio.NewWriter(rb)
	payload := []byte("hello world")
	n, err := writer.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes written, got %d", len(payload), n)
	}

	// Flush 前 RingBuffer 应该为空（数据在 bufio 内部缓冲区）
	if rb.Length() != 0 {
		t.Fatalf("ringbuffer should be empty before flush, got %d", rb.Length())
	}

	// Flush 后数据才进入 RingBuffer
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if rb.Length() != len(payload) {
		t.Fatalf("ringbuffer should have %d bytes after flush, got %d", len(payload), rb.Length())
	}

	// 从 RingBuffer 读出验证
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(rb, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", string(buf))
	}
}

// TestBufioWriter_MultipleFlush 验证 bufio.Writer 多次 Write + Flush
func TestBufioWriter_MultipleFlush(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	writer := bufio.NewWriter(rb)
	writer.Write([]byte("chunk1-"))
	writer.Flush()
	writer.Write([]byte("chunk2-"))
	writer.Flush()
	writer.Write([]byte("chunk3"))
	writer.Flush()

	expected := "chunk1-chunk2-chunk3"
	reader := bufio.NewReader(rb)
	result := make([]byte, len(expected))
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}

	if string(result) != expected {
		t.Fatalf("expected '%s', got '%s'", expected, string(result))
	}
}

// TestBufioWriter_AutoFlush 验证 bufio.Writer 缓冲区满时自动 Flush
func TestBufioWriter_AutoFlush(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	// 使用 512 字节的 bufio.Writer 缓冲区，写入 2000 字节触发自动 Flush
	writer := bufio.NewWriterSize(rb, 512)

	// 分块写入，确保累计超过 bufio 缓冲区
	writer.Write(bytes.Repeat([]byte("A"), 400))
	// 此时 400 < 512，数据在 bufio 内部缓冲
	if rb.Length() != 0 {
		t.Fatalf("ringbuffer should be empty before auto-flush, got %d", rb.Length())
	}

	writer.Write(bytes.Repeat([]byte("B"), 400))
	// 800 > 512，第一次 400 字节应该已被自动 Flush
	if rb.Length() < 400 {
		t.Fatalf("ringbuffer should have at least 400 bytes after auto-flush, got %d", rb.Length())
	}

	writer.Flush()

	// 全部数据应可读取
	reader := bufio.NewReader(rb)
	result := make([]byte, 800)
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(result, bytes.Repeat([]byte("A"), 400)) {
		t.Fatal("data corruption: first chunk should be all 'A's")
	}
}

// =============================================================================
// bufio.Reader.Peek 与 RingBuffer 交互测试（核心场景）
// =============================================================================

// TestPeek_SmallerThanBufioBuffer 当 Peek 请求小于 bufio 内部缓冲区时，
// bufio 从 RingBuffer 预取数据到内部缓冲区，但 Peek 不消费数据。
// RingBuffer 的数据会被移入 bufio 的内部缓冲区。
func TestPeek_SmallerThanBufioBuffer(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	payload := []byte("hello peek test world")
	rb.Write(payload)

	initialLen := rb.Length()
	t.Logf("initial ringbuffer length: %d", initialLen)

	reader := bufio.NewReader(rb)

	// Peek 5 字节 —— 这会让 bufio 填充其内部缓冲区（默认 4096 字节）
	peeked, err := reader.Peek(5)
	if err != nil {
		t.Fatal(err)
	}
	if string(peeked) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", string(peeked))
	}

	// 关键：Peek 后 RingBuffer 中的数据已被 bufio 读取到内部缓冲区
	// RingBuffer.Length() 会减少（全部数据已被预取到 bufio 内部缓冲）
	afterPeekLen := rb.Length()
	t.Logf("ringbuffer length after Peek(5): %d", afterPeekLen)
	if afterPeekLen != 0 {
		// 数据量小于 bufio 默认缓冲区大小，所有数据都被预取走了
		t.Logf("ringbuffer has %d bytes remaining (all data was buffered by bufio)", afterPeekLen)
	}

	// 再次 Peek 相同数据，应该返回相同结果
	peeked2, err := reader.Peek(5)
	if err != nil {
		t.Fatal(err)
	}
	if string(peeked2) != "hello" {
		t.Fatalf("second peek expected 'hello', got '%s'", string(peeked2))
	}

	// 读取 5 字节后，Peek 光标前进
	discard := make([]byte, 5)
	io.ReadFull(reader, discard)

	// 后续 Peek 应返回新位置的数据
	peeked3, err := reader.Peek(4)
	if err != nil {
		t.Fatal(err)
	}
	if string(peeked3) != " pee" {
		t.Fatalf("expected ' pee', got '%s'", string(peeked3))
	}
}

// TestPeek_ExceedsBufioBuffer 当 Peek 请求大于 bufio 内部缓冲区时，
// bufio.Reader.Peek 返回 bufio.ErrBufferFull
func TestPeek_ExceedsBufioBuffer(t *testing.T) {
	rb := ringbuffer.New(8192)
	defer rb.CloseWithError(nil)

	// 写入 3000 字节数据
	payload := bytes.Repeat([]byte("ABC"), 1000) // 3000 bytes
	rb.Write(payload)

	// 使用 512 字节的 bufio 内部缓冲区
	reader := bufio.NewReaderSize(rb, 512)

	// Peek 2000 字节 —— 超过 bufio 内部缓冲区，应返回 ErrBufferFull
	_, err := reader.Peek(2000)
	if err != bufio.ErrBufferFull {
		t.Fatalf("expected bufio.ErrBufferFull, got %v", err)
	}
	t.Logf("Peek(2000) with 512 buffer correctly returned ErrBufferFull")

	// Peek 不超过缓冲区大小应该成功
	peeked, err := reader.Peek(512)
	if err != nil {
		t.Fatal(err)
	}
	if len(peeked) != 512 {
		t.Fatalf("expected 512 bytes peeked, got %d", len(peeked))
	}

	// 数据完整性：从 bufio.Reader 读出全部数据
	result := make([]byte, 3000)
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, payload) {
		t.Fatal("data mismatch after full read")
	}
}

// TestPeek_BufioBuffered 验证 bufio.Reader.Buffered() 方法的行为
// Buffered() 返回 bufio 内部缓冲区中可读的字节数
func TestPeek_BufioBuffered(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("abcdefghij"))

	reader := bufio.NewReader(rb)

	// 初始 Buffered 为 0
	if n := reader.Buffered(); n != 0 {
		t.Fatalf("expected 0 buffered initially, got %d", n)
	}

	// Peek 3 字节：bufio 从 ringbuffer 读取 4096（默认缓冲大小）到内部缓冲
	_, err := reader.Peek(3)
	if err != nil {
		t.Fatal(err)
	}

	// Buffered 现在应该是 10（ringbuffer 中的全部数据都被预取）
	buffered := reader.Buffered()
	t.Logf("buffered after Peek(3): %d", buffered)
	if buffered != 10 {
		t.Fatalf("expected 10 bytes buffered, got %d", buffered)
	}

	// RingBuffer 应该为空
	if rb.Length() != 0 {
		t.Fatalf("ringbuffer should be empty, got %d", rb.Length())
	}
}

// TestPeek_NoAvailableData 验证 RingBuffer 为空时 bufio.Reader.Peek 的行为
func TestPeek_NoAvailableData(t *testing.T) {
	rb := ringbuffer.New(1024)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)

	// 非阻塞模式下，RingBuffer 无数据时读应返回错误
	_, err := reader.Peek(10)
	if err == nil {
		t.Fatal("expected error on empty non-blocking ringbuffer")
	}
	t.Logf("expected error on empty buffer: %v", err)
}

// TestPeek_BlockingRingBuffer 验证阻塞模式 RingBuffer + bufio.Reader.Peek
func TestPeek_BlockingRingBuffer(t *testing.T) {
	rb := ringbuffer.New(1024)
	rb.SetBlocking(true)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)

	// 在另一个 goroutine 中延迟写入
	var peekErr error
	var peeked []byte
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		rb.Write([]byte("delayed data"))
	}()

	// Peek 会阻塞等待数据
	peeked, peekErr = reader.Peek(5)
	wg.Wait()

	if peekErr != nil {
		t.Fatal(peekErr)
	}
	if string(peeked) != "delay"[0:5] {
		t.Fatalf("expected 'delay', got '%s'", string(peeked))
	}
}

// TestPeek_ContextCancellation 验证 CloseWithError 对 bufio.Reader.Peek 的影响
func TestPeek_ContextCancellation(t *testing.T) {
	rb := ringbuffer.New(1024)
	rb.SetBlocking(true)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)

	// Peek 在后台 goroutine 中阻塞，通过 CloseWithError 取消
	errCh := make(chan error, 1)
	go func() {
		_, err := reader.Peek(10)
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)
	rb.CloseWithError(io.ErrUnexpectedEOF)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after close")
		}
		t.Logf("got expected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Peek to return after CloseWithError")
	}
}

// =============================================================================
// peekCombined 模式测试（项目中的实际使用模式）
// =============================================================================

// TestPeekCombined_NoBufioData 验证 peekCombined：bufio.Reader 无缓存时只返回 RingBuffer 数据
func TestPeekCombined_NoBufioData(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	payload := []byte("protocol detection data")
	rb.Write(payload)

	reader := bufio.NewReader(rb)

	// bufio.Reader 未读取任何数据，Buffered() == 0
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}
	if string(combined) != string(payload) {
		t.Fatalf("expected '%s', got '%s'", string(payload), string(combined))
	}
	if reader.Buffered() != 0 {
		t.Fatalf("bufio should have 0 buffered, got %d", reader.Buffered())
	}
}

// TestPeekCombined_WithBufioData 验证 peekCombined：bufio.Reader 有缓存时合并数据
func TestPeekCombined_WithBufioData(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	// 先写入 100 字节
	firstChunk := bytes.Repeat([]byte("X"), 100)
	rb.Write(firstChunk)

	reader := bufio.NewReader(rb)

	// 触发 bufio 从 RingBuffer 预取数据（Peek 50 字节但不消费）
	if _, err := reader.Peek(50); err != nil {
		t.Fatal(err)
	}

	// 此时 bufio 内部缓存了全部 100 字节，RingBuffer 为空
	t.Logf("buffered: %d, ringbuffer length: %d", reader.Buffered(), rb.Length())

	// peekCombined 合并 bufio 缓存 + RingBuffer 数据
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}

	// 合并后应包含 bufio 中的数据 + RingBuffer 中的数据（共 100 字节）
	if len(combined) != 100 {
		t.Fatalf("expected 100 bytes combined, got %d", len(combined))
	}

	// 数据未被消费，可以完整读出
	result := make([]byte, 100)
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, firstChunk) {
		t.Fatal("data mismatch after peekCombined")
	}
}

// TestPeekCombined_DataSplitBetweenBufioAndRingbuffer
// 验证数据分散在 bufio 内部缓冲区和 RingBuffer 之间的合并场景
func TestPeekCombined_DataSplitBetweenBufioAndRingbuffer(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	// 写入 3000 字节
	allData := bytes.Repeat([]byte("ABCD"), 750) // 3000 bytes
	rb.Write(allData)

	// 使用 512 字节的 bufio 缓冲区
	reader := bufio.NewReaderSize(rb, 512)

	// 只读取 100 字节，触发 bufio 填充 512 字节内部缓冲
	partial := make([]byte, 100)
	if _, err := io.ReadFull(reader, partial); err != nil {
		t.Fatal(err)
	}

	// 此时 bufio 内部缓冲还有 412 字节，RingBuffer 还有 3000 - 512 = 2488 字节
	t.Logf("after reading 100 bytes: buffered=%d, ringbuffer=%d", reader.Buffered(), rb.Length())

	// peekCombined 应合并两者
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}

	expectedRemaining := len(allData) - 100 // 2900
	if len(combined) != expectedRemaining {
		t.Fatalf("expected %d bytes combined, got %d", expectedRemaining, len(combined))
	}

	// 验证 peekCombined 返回的数据内容正确
	expectedData := allData[100:]
	if !bytes.Equal(combined, expectedData) {
		t.Fatal("combined data does not match expected remaining data")
	}
}

// TestPeekCombined_PartiallyConsumed 验证消费部分数据后 peekCombined 的正确性
func TestPeekCombined_PartiallyConsumed(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	payload := []byte("GET /index.html HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rb.Write(payload)

	reader := bufio.NewReaderSize(rb, 64) // 小缓冲区强制数据跨 bufio 和 ringbuffer

	// 读取第一行
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first line: %s", strings.TrimRight(line, "\r\n"))

	// peekCombined 应能查出剩余数据
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}

	// 验证剩余数据可以完整读出
	remaining := make([]byte, len(payload)-len(line))
	if _, err := io.ReadFull(reader, remaining); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(combined, remaining) {
		t.Fatalf("peekCombined result does not match actual remaining data\ncombined: %q\nactual:  %q", combined, remaining)
	}
}

// =============================================================================
// RingBuffer 对象池集成测试
// =============================================================================

// TestRingBufferPool_Basic 验证对象池获取和释放的基本流程
func TestRingBufferPool_Basic(t *testing.T) {
	rb := getRingBuffer()
	if rb == nil {
		t.Fatal("getRingBuffer returned nil")
	}
	if rb.Length() != 0 {
		t.Fatalf("fresh ringbuffer should have length 0, got %d", rb.Length())
	}
	if rb.Capacity() != RING_BUFFER_SIZE {
		t.Fatalf("expected capacity %d, got %d", RING_BUFFER_SIZE, rb.Capacity())
	}

	rb.Write([]byte("test"))
	releaseRingBuffer(rb)

	// 再次获取，应被 Reset
	rb2 := getRingBuffer()
	if rb2.Length() != 0 {
		t.Fatalf("reused ringbuffer should be reset to 0, got %d", rb2.Length())
	}
	releaseRingBuffer(rb2)
}

// TestRingBufferPool_Concurrent 验证并发获取和释放
func TestRingBufferPool_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb := getRingBuffer()
			rb.Write([]byte("concurrent"))
			time.Sleep(time.Millisecond)
			releaseRingBuffer(rb)
		}()
	}
	wg.Wait()
}

// =============================================================================
// 生产环境场景模拟测试
// =============================================================================

// TestProductionScenario_StreamingProtocol 模拟流式协议读取
// 数据陆续写入 RingBuffer，bufio.Reader 逐步读取
func TestProductionScenario_StreamingProtocol(t *testing.T) {
	rb := ringbuffer.New(4096)
	rb.SetBlocking(true)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)

	// 模拟数据陆续到达
	go func() {
		time.Sleep(10 * time.Millisecond)
		rb.Write([]byte("HTTP/1.1 200 OK\r\n"))
		time.Sleep(10 * time.Millisecond)
		rb.Write([]byte("Content-Length: 5\r\n"))
		time.Sleep(10 * time.Millisecond)
		rb.Write([]byte("\r\n"))
		rb.Write([]byte("Hello"))
	}()

	// 读取状态行
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if statusLine != "HTTP/1.1 200 OK\r\n" {
		t.Fatalf("unexpected status line: '%s'", statusLine)
	}

	// 读取 Content-Length 头
	headerLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if headerLine != "Content-Length: 5\r\n" {
		t.Fatalf("unexpected header: '%s'", headerLine)
	}

	// 读取空行
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// 读取 body
	body := make([]byte, 5)
	if _, err := io.ReadFull(reader, body); err != nil {
		t.Fatal(err)
	}
	if string(body) != "Hello" {
		t.Fatalf("unexpected body: '%s'", string(body))
	}
}

// TestProductionScenario_PeekThenConsume 模拟 peekCombined 协议检测后再消费的完整流程
func TestProductionScenario_PeekThenConsume(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	// 模拟收到一段 HTTP 请求数据
	httpPayload := []byte("GET /api/v1/users HTTP/1.1\r\nHost: localhost\r\n\r\n")
	rb.Write(httpPayload)

	reader := bufio.NewReader(rb)

	// 第一步：peekCombined 协议检测
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}

	// 检测是否为 HTTP 协议
	isHTTP := bytes.HasPrefix(combined, []byte("GET ")) ||
		bytes.HasPrefix(combined, []byte("POST ")) ||
		bytes.HasPrefix(combined, []byte("HTTP/"))
	if !isHTTP {
		t.Fatal("expected HTTP protocol detected")
	}

	// 第二步：协议检测后正常消费数据
	// 注意：由于 bufio.Reader.Buffer 默认 4096 > 数据量，peekCombined 中 Peek(Buffered())
	// 已把数据全部读入 bufio 内部缓冲，RingBuffer 现在为空
	// 缓冲区中数据必须从 bufio.Reader 读取
	result := make([]byte, len(httpPayload))
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, httpPayload) {
		t.Fatal("data mismatch after consume")
	}
}

// =============================================================================
// 边界条件测试
// =============================================================================

// TestEdgeCase_RingBufferFull 验证 RingBuffer 写满时的行为
func TestEdgeCase_RingBufferFull(t *testing.T) {
	rb := ringbuffer.New(64)
	rb.SetBlocking(false) // 非阻塞避免死锁
	defer rb.CloseWithError(nil)

	// 写满 buffer
	data := bytes.Repeat([]byte("X"), 64)
	n, err := rb.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 64 {
		t.Fatalf("expected 64 bytes written, got %d", n)
	}

	// 再次写入应失败
	_, err = rb.Write([]byte("Y"))
	if err == nil {
		t.Fatal("expected error when writing to full ringbuffer")
	}
	t.Logf("write to full buffer error: %v", err)
}

// TestEdgeCase_BufioReader_Discard 验证 bufio.Reader.Discard 与 RingBuffer 交互
func TestEdgeCase_BufioReader_Discard(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("prefix_important_data"))

	reader := bufio.NewReader(rb)

	// Discard 7 字节 ("prefix_")
	discarded, err := reader.Discard(7)
	if err != nil {
		t.Fatal(err)
	}
	if discarded != 7 {
		t.Fatalf("expected 7 discarded, got %d", discarded)
	}

	// 剩余数据应正确
	remaining := make([]byte, 14)
	if _, err := io.ReadFull(reader, remaining); err != nil {
		t.Fatal(err)
	}
	if string(remaining) != "important_data" {
		t.Fatalf("expected 'important_data', got '%s'", string(remaining))
	}
}

// TestEdgeCase_ReadExactBoundary 验证读取刚好等于 bufio 缓冲区边界的数据
func TestEdgeCase_ReadExactBoundary(t *testing.T) {
	rb := ringbuffer.New(10240)
	defer rb.CloseWithError(nil)

	// 写入刚好两倍 bufio 默认缓冲区的数据
	data := bytes.Repeat([]byte("Z"), 8192) // 2 * 4096
	rb.Write(data)

	reader := bufio.NewReader(rb)
	result := make([]byte, 8192)
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		t.Fatal("data mismatch")
	}
}

// TestEdgeCase_EmptyRingBufferRead 验证空 RingBuffer 的读取行为
func TestEdgeCase_EmptyRingBufferRead(t *testing.T) {
	rb := ringbuffer.New(1024)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)

	// 非阻塞空 buffer 读应返回错误
	_, err := reader.ReadByte()
	if err == nil {
		t.Fatal("expected error on empty ringbuffer")
	}
	t.Logf("expected error reading from empty buffer: %v", err)
}

// TestEdgeCase_WriteAfterClose 验证关闭后写入
func TestEdgeCase_WriteAfterClose(t *testing.T) {
	rb := ringbuffer.New(1024)
	rb.CloseWithError(io.ErrClosedPipe)

	_, err := rb.Write([]byte("data"))
	if err == nil {
		t.Fatal("expected error writing to closed ringbuffer")
	}
}

// TestEdgeCase_ResetBehavior 验证 Reset 后 RingBuffer 状态
func TestEdgeCase_ResetBehavior(t *testing.T) {
	rb := ringbuffer.New(1024)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("some data"))
	rb.Reset()

	if rb.Length() != 0 {
		t.Fatalf("after reset, length should be 0, got %d", rb.Length())
	}

	// Reset 后应可以正常写入和读取
	rb.Write([]byte("new data"))
	rb.SetBlocking(false)
	buf := make([]byte, 8)
	n, err := rb.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "new data" {
		t.Fatalf("expected 'new data', got '%s'", string(buf[:n]))
	}
}

// TestEdgeCase_IsEmpty_IsFull 验证 Empty/Full 状态判断
func TestEdgeCase_IsEmpty_IsFull(t *testing.T) {
	rb := ringbuffer.New(16)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	if !rb.IsEmpty() {
		t.Fatal("expected empty ringbuffer")
	}

	rb.Write(bytes.Repeat([]byte("A"), 16))
	if !rb.IsFull() {
		t.Fatal("expected full ringbuffer")
	}

	// 读取后恢复空闲
	discard := make([]byte, 8)
	rb.Read(discard)
	if rb.IsFull() {
		t.Fatal("should not be full after reading 8 bytes")
	}

	rb.Reset()
	if !rb.IsEmpty() {
		t.Fatal("expected empty after reset")
	}
}

// TestEdgeCase_Free_vs_Length 验证 Free 和 Length 的关系
func TestEdgeCase_Free_vs_Length(t *testing.T) {
	rb := ringbuffer.New(100)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	if free := rb.Free(); free != 100 {
		t.Fatalf("expected 100 free, got %d", free)
	}

	rb.Write(bytes.Repeat([]byte("X"), 40))
	if rb.Length() != 40 {
		t.Fatalf("expected length 40, got %d", rb.Length())
	}
	if rb.Free() != 60 {
		t.Fatalf("expected 60 free, got %d", rb.Free())
	}
	if rb.Length()+rb.Free() != rb.Capacity() {
		t.Fatal("length + free != capacity")
	}
}

// =============================================================================
// 多协议检测场景测试
// =============================================================================

// TestMultiProtocol_ChainedPeek 模拟连续 Peek 不同大小用于协议检测
func TestMultiProtocol_ChainedPeek(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	payload := []byte{
		0x16, 0x03, 0x01, 0x00, 0xA1, // TLS ClientHello 头 5 字节
	}
	// 附加一些数据
	payload = append(payload, bytes.Repeat([]byte{0x00}, 100)...)
	rb.Write(payload)

	reader := bufio.NewReader(rb)

	// 先 Peek 1 字节检测 TLS record type
	peek1, _ := reader.Peek(1)
	isTLS := peek1[0] == 0x16
	if !isTLS {
		t.Fatal("expected TLS record type 0x16")
	}

	// 再 Peek 5 字节获取 TLS record header
	peek5, _ := reader.Peek(5)
	t.Logf("TLS header: %x", peek5)

	// 连续 Peek 不影响数据完整性
	result := make([]byte, len(payload))
	if _, err := io.ReadFull(reader, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, payload) {
		t.Fatal("data mismatch after chained peeks")
	}
}

// TestMultiProtocol_PeekAndSwitchProtocol 模拟 Peek 后切换到不同协议解析器
func TestMultiProtocol_PeekAndSwitchProtocol(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	rb.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 13\r\n\r\nHello, World!"))

	reader := bufio.NewReader(rb)

	// 协议检测
	capBuf := make([]byte, rb.Capacity())
	combined, err := peekCombined(reader, rb, capBuf)
	if err != nil {
		t.Fatal(err)
	}

	// 检测为 HTTP 响应
	if !bytes.HasPrefix(combined, []byte("HTTP/")) {
		t.Fatal("expected HTTP protocol")
	}

	// 正常解析 HTTP
	statusLine, _ := reader.ReadString('\n')
	headerLine, _ := reader.ReadString('\n')
	reader.ReadString('\n') // empty line

	t.Logf("status: %s", strings.TrimRight(statusLine, "\r\n"))
	t.Logf("header: %s", strings.TrimRight(headerLine, "\r\n"))

	body := make([]byte, 13)
	io.ReadFull(reader, body)
	if string(body) != "Hello, World!" {
		t.Fatalf("expected 'Hello, World!', got '%s'", string(body))
	}
}

// =============================================================================
// bufio.Reader + bufio.Writer 组合管道测试
// =============================================================================

// TestBufioPipeline_ReaderWriter 验证 bufio.Writer -> RingBuffer -> bufio.Reader 管道
func TestBufioPipeline_ReaderWriter(t *testing.T) {
	rb := ringbuffer.New(4096)
	defer rb.CloseWithError(nil)

	writer := bufio.NewWriter(rb)
	reader := bufio.NewReader(rb)

	// 写入并刷新
	writer.Write([]byte("pipeline test data\n"))
	writer.Flush()

	// 从 reader 读取
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if line != "pipeline test data\n" {
		t.Fatalf("expected 'pipeline test data\\n', got '%s'", line)
	}
}

// TestBufioPipeline_ConcurrentReadWrite 验证并发读写管道
func TestBufioPipeline_ConcurrentReadWrite(t *testing.T) {
	rb := ringbuffer.New(65536)
	rb.SetBlocking(true)
	defer rb.CloseWithError(nil)

	reader := bufio.NewReader(rb)
	writer := bufio.NewWriter(rb)

	messages := []string{"msg1\n", "msg2\n", "msg3\n", "msg4\n", "msg5\n"}

	// 写协程
	var wg sync.WaitGroup
	wg.Add(2)

	errCh := make(chan error, 2)

	go func() {
		defer wg.Done()
		for _, msg := range messages {
			if _, err := writer.Write([]byte(msg)); err != nil {
				errCh <- err
				return
			}
			if err := writer.Flush(); err != nil {
				errCh <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 读协程
	go func() {
		defer wg.Done()
		for _, expected := range messages {
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			if line != expected {
				errCh <- err
				return
			}
		}
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// =============================================================================
// 基准测试
// =============================================================================

// BenchmarkBufioReader_ReadOverRingBuffer 基准：bufio.Reader 从 RingBuffer 读取
func BenchmarkBufioReader_ReadOverRingBuffer(b *testing.B) {
	rb := ringbuffer.New(256 * 1024)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	data := bytes.Repeat([]byte("X"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(data)
		reader := bufio.NewReader(rb)
		buf := make([]byte, 1024)
		io.ReadFull(reader, buf)
	}
}

// BenchmarkBufioReader_PeekOverRingBuffer 基准：bufio.Reader.Peek 从 RingBuffer
func BenchmarkBufioReader_PeekOverRingBuffer(b *testing.B) {
	rb := ringbuffer.New(256 * 1024)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	data := bytes.Repeat([]byte("X"), 4096)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(data)
		reader := bufio.NewReader(rb)
		reader.Peek(64)
	}
}

// BenchmarkPeekCombined 基准：peekCombined 模式
func BenchmarkPeekCombined(b *testing.B) {
	rb := ringbuffer.New(256 * 1024)
	rb.SetBlocking(false)
	defer rb.CloseWithError(nil)

	data := bytes.Repeat([]byte("X"), 4096)
	capBuf := make([]byte, rb.Capacity())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Write(data)
		reader := bufio.NewReader(rb)
		reader.Peek(100) // 先让 bufio 预取一些数据
		peekCombined(reader, rb, capBuf)
	}
}

// BenchmarkGetRingBuffer_Pooled 基准：从对象池获取 RingBuffer
func BenchmarkGetRingBuffer_Pooled(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb := getRingBuffer()
		releaseRingBuffer(rb)
	}
}

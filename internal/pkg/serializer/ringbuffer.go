package serializer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/smallnest/ringbuffer"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type BufferSize int

const (
	SmallBufferSzie  BufferSize = 1024 * 8
	MiddleBufferSize BufferSize = SmallBufferSzie * 8
	LargeBufferSzie  BufferSize = MiddleBufferSize * 8
)

var (
	smallBufferPool = sync.Pool{
		New: func() any {
			return ringbuffer.New(int(SmallBufferSzie))
		},
	}
	middleBufferPool = sync.Pool{
		New: func() any {
			return ringbuffer.New(int(MiddleBufferSize))
		},
	}
	largeBufferPool = sync.Pool{
		New: func() any {
			return ringbuffer.New(int(LargeBufferSzie))
		},
	}
)

// RingBufferStore 是环形缓冲区的序列化表示结构，用于JSON序列化和持久化存储
// 该结构体将缓冲区的状态和数据编码为可序列化的格式，便于网络传输和文件存储
type RingBufferStore struct {
	MaxSize int    `json:"max_size"` // 缓冲区的最大容量，单位：字节
	Data    string `json:"buffer"`   // 缓冲区数据的base64编码字符串
}

// ToBuffer 将存储表示转换为环形缓冲区实例
// 该方法解码base64数据并重新构建环形缓冲区，恢复缓冲区的完整状态
// 返回值: 新创建的RingBuffer实例，包含原始数据和容量
func (s *RingBufferStore) ToBuffer() *RingBuffer {
	raw, err := base64.StdEncoding.DecodeString(s.Data)
	if err != nil {
		panic(err)
	}
	data := make([]byte, len(raw))
	copy(data, raw)
	inner := ringbuffer.New(s.MaxSize)
	if _, err := inner.Write(data); err != nil {
		panic(err)
	}
	return &RingBuffer{
		inner: inner,
	}
}

// RingBuffer 实现了一个线程安全的环形缓冲区包装器
// 基于github.com/smallnest/ringbuffer实现，支持并发读写操作
// 适用于生产者-消费者模式、日志缓冲、数据流处理等场景
type RingBuffer struct {
	lock  *sync.Once             `json:"-"`
	size  BufferSize             `json:"-"`
	inner *ringbuffer.RingBuffer `json:"-"` // 内部环形缓冲区实例
}

// NewRingBuffer 创建并初始化一个新的环形缓冲区
// ctx: 上下文，用于取消操作或设置超时
// size: 缓冲区的最大容量，必须为正整数
// timeout: 操作超时时间，用于设置读写超时
// blocking: 是否阻塞读写操作，true表示阻塞，false表示非阻塞
// 返回值: 新创建的RingBuffer实例，初始状态为空
func NewRingBuffer(size BufferSize) *RingBuffer {
	var inner *ringbuffer.RingBuffer
	switch size {
	case SmallBufferSzie:
		inner = smallBufferPool.Get().(*ringbuffer.RingBuffer)
	case MiddleBufferSize:
		inner = middleBufferPool.Get().(*ringbuffer.RingBuffer)
	case LargeBufferSzie:
		inner = largeBufferPool.Get().(*ringbuffer.RingBuffer)
	default:
		inner = ringbuffer.New(int(size))
	}
	inner.Reset()
	return &RingBuffer{
		lock:  &sync.Once{},
		inner: inner,
		size:  size,
	}
}

func (r *RingBuffer) WithTimeout(timeout time.Duration) *RingBuffer {
	r.inner = r.inner.WithTimeout(timeout)
	return r
}

func (r *RingBuffer) WithCancel(ctx context.Context) *RingBuffer {
	r.inner = r.inner.WithCancel(ctx)
	return r
}

func (r *RingBuffer) SetBlocking(blocking bool) *RingBuffer {
	r.inner = r.inner.SetBlocking(blocking)
	return r
}

func (r *RingBuffer) Release() {
	r.lock.Do(func() {
		if r.inner == nil {
			return
		}
		r.inner.Reset()
		switch r.size {
		case SmallBufferSzie:
			smallBufferPool.Put(r.inner)
		case MiddleBufferSize:
			middleBufferPool.Put(r.inner)
		case LargeBufferSzie:
			largeBufferPool.Put(r.inner)
		}
		r.inner = nil
	})
}

// ToStore 将环形缓冲区转换为可序列化的存储表示
// 该方法提取缓冲区当前的所有数据，并连同容量信息一起编码为存储格式
// 返回值: 包含缓冲区完整状态和数据的RingBufferStore实例
func (r *RingBuffer) ToStore() *RingBufferStore {
	maxSize := r.inner.Capacity()
	data := make([]byte, maxSize, maxSize)
	if _, err := r.inner.Peek(data); err != nil {
		panic(err)
	}
	return &RingBufferStore{
		MaxSize: maxSize,
		Data:    base64.StdEncoding.EncodeToString(data),
	}
}

// MarshalJSON 实现json.Marshaler接口，将环形缓冲区序列化为JSON格式
// 该方法将环形缓冲区转换为RingBufferStore结构，再编码为JSON字节流
// 返回值: JSON编码的字节切片和序列化过程中的任何错误
func (r RingBuffer) MarshalJSON() ([]byte, error) {
	bytes, err := json.Marshal(r.ToStore())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return bytes, nil
}

// UnmarshalJSON 实现json.Unmarshaler接口，从JSON数据反序列化环形缓冲区
// 该方法解析JSON数据为RingBufferStore结构，然后重建环形缓冲区实例
// data: 包含序列化缓冲区状态的JSON编码字节切片
// 返回值: 反序列化过程中的错误，如果成功则返回nil
func (r *RingBuffer) UnmarshalJSON(data []byte) error {
	var store RingBufferStore
	if err := json.Unmarshal(data, &store); err != nil {
		return errors.WithStack(err)
	}
	*r = *store.ToBuffer()
	return nil
}

// Flush 重置环形缓冲区，清空所有数据并将读写指针归零
// 该方法将缓冲区的长度重置为0，但不改变容量，可以立即重用缓冲区
// 返回值: 操作过程中的错误，通常为nil
func (r *RingBuffer) Flush() error {
	return r.inner.Flush()
}

// Read 线程安全的读取方法，从环形缓冲区读取数据到提供的切片中
// 该方法从缓冲区头部读取数据，读取后数据从缓冲区中移除，读取指针向前移动
// data: 用于存储读取结果的字节切片，切片容量决定最大读取量
// 返回值: 实际读取的字节数和读取过程中的错误
func (r *RingBuffer) Read(data []byte) (int, error) {
	return r.inner.Read(data)
}

// ReadAll 读取环形缓冲区中的所有数据，返回完整的字节切片
// 该方法一次性读取缓冲区中的所有可用数据，读取后缓冲区变为空状态
// 返回值: 包含所有数据的字节切片和读取过程中的错误
func (r *RingBuffer) ReadAll() ([]byte, error) {
	length := r.inner.Length()
	result := make([]byte, length)
	if _, err := r.inner.Read(result); err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}

// Write 线程安全的写入方法，向环形缓冲区写入数据
// 该方法向缓冲区尾部写入数据，如果缓冲区已满，部分数据可能被丢弃
// data: 要写入缓冲区的字节切片
// 返回值: 实际写入的字节数和写入过程中的错误
func (r *RingBuffer) Write(data []byte) (int, error) {
	return r.inner.Write(data)
}

// Peek 查看环形缓冲区中的数据，但不移动读取指针
// 该方法允许查看缓冲区头部的数据，读取指针保持不变，数据不会被消耗
// data: 用于存储查看结果的字节切片，切片容量决定最大查看量
// 返回值: 实际查看的字节数和操作过程中的错误
func (r *RingBuffer) Peek(data []byte) (int, error) {
	return r.inner.Peek(data)
}

// PeekAll 查看环形缓冲区中的所有数据，返回完整的字节切片
// 该方法查看缓冲区中的所有可用数据，但不移动读取指针，数据保持原样
// 返回值: 包含所有数据的字节切片和操作过程中的错误
func (r *RingBuffer) PeekAll(data []byte) ([]byte, error) {
	length := r.inner.Length()
	data = data[:length]
	if length == 0 {
		return data, nil
	}
	if _, err := r.inner.Peek(data); err != nil {
		return nil, errors.WithStack(err)
	}
	return data, nil
}

// Capacity 返回环形缓冲区的总容量
// 该方法线程安全，返回缓冲区创建时指定的最大容量值
// 返回值: 缓冲区的总容量，单位：字节
func (r *RingBuffer) Capacity() int {
	return r.inner.Capacity()
}

// Length 返回环形缓冲区中当前可读取的数据长度
// 该方法线程安全，返回缓冲区中已存储但尚未读取的字节数
// 返回值: 可读取的字节数，范围从0到Capacity()
func (r *RingBuffer) Length() int {
	return r.inner.Length()
}

// Free 返回环形缓冲区中剩余的可写入空间
// 该方法线程安全，计算缓冲区中还可以写入的字节数
// 返回值: 剩余的可写入字节数，等于Capacity() - Length()
func (r *RingBuffer) Free() int {
	return r.inner.Free()
}

func (r *RingBuffer) IsEmpty() bool {
	return r.inner.IsEmpty()
}

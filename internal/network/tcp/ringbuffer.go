package tcp

import (
	"sync"

	"github.com/smallnest/ringbuffer"
)

const (
	RING_BUFFER_SIZE = 256 * 1024
)

var (
	bufferPool = sync.Pool{
		New: func() any {
			return ringbuffer.New(RING_BUFFER_SIZE)
		},
	}
)

func getRingBuffer() *ringbuffer.RingBuffer {
	rb := bufferPool.Get().(*ringbuffer.RingBuffer)
	rb.Reset()
	return rb
}

func releaseRingBuffer(rb *ringbuffer.RingBuffer) {
	bufferPool.Put(rb)
}

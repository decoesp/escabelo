package engine

import "sync"

// BufferPool provides reusable byte slices to reduce allocations
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, size)
				return &buf
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() *[]byte {
	return bp.pool.Get().(*[]byte)
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buf *[]byte) {
	bp.pool.Put(buf)
}

// Global buffer pools for common sizes
var (
	SmallBufferPool  = NewBufferPool(4096)    // 4KB
	MediumBufferPool = NewBufferPool(65536)   // 64KB
	LargeBufferPool  = NewBufferPool(1048576) // 1MB
)

package gozstd

// This file consolidates:
// - resource_management.go
// - managed_buffer.go  
// - buffer_pool.go
// - threadpool.go

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrPoolClosed is returned when trying to use a closed pool
	ErrPoolClosed = errors.New("pool is closed")
)

// ===== Resource Management =====

// ResourceManager manages compression/decompression resources with automatic cleanup
type ResourceManager struct {
	contexts map[interface{}]time.Time
	mu       sync.RWMutex
	ticker   *time.Ticker
	done     chan bool
}

// NewResourceManager creates a new resource manager
func NewResourceManager() *ResourceManager {
	rm := &ResourceManager{
		contexts: make(map[interface{}]time.Time),
		ticker:   time.NewTicker(5 * time.Minute),
		done:     make(chan bool),
	}
	
	// Start cleanup goroutine
	go rm.cleanup()
	
	return rm
}

func (rm *ResourceManager) cleanup() {
	for {
		select {
		case <-rm.ticker.C:
			rm.cleanupStale()
		case <-rm.done:
			return
		}
	}
}

func (rm *ResourceManager) cleanupStale() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	now := time.Now()
	for ctx, lastUsed := range rm.contexts {
		if now.Sub(lastUsed) > 10*time.Minute {
			// Resource hasn't been used for 10 minutes, release it
			switch v := ctx.(type) {
			case *CCtx:
				v.Release()
			case *CDict:
				v.Release()
			case *DDict:
				v.Release()
			}
			delete(rm.contexts, ctx)
		}
	}
}

// Track tracks a resource for automatic cleanup
func (rm *ResourceManager) Track(resource interface{}) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.contexts[resource] = time.Now()
}

// Touch updates the last used time for a resource
func (rm *ResourceManager) Touch(resource interface{}) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, exists := rm.contexts[resource]; exists {
		rm.contexts[resource] = time.Now()
	}
}

// Release immediately releases a resource
func (rm *ResourceManager) Release(resource interface{}) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	switch v := resource.(type) {
	case *CCtx:
		v.Release()
	case *CDict:
		v.Release()
	case *DDict:
		v.Release()
	}
	
	delete(rm.contexts, resource)
}

// Stop stops the resource manager
func (rm *ResourceManager) Stop() {
	rm.ticker.Stop()
	close(rm.done)
	
	// Release all tracked resources
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	for ctx := range rm.contexts {
		switch v := ctx.(type) {
		case *CCtx:
			v.Release()
		case *CDict:
			v.Release()
		case *DDict:
			v.Release()
		}
	}
	
	rm.contexts = make(map[interface{}]time.Time)
}

// GlobalResourceManager is the global resource manager instance
var GlobalResourceManager = NewResourceManager()

// ===== Buffer Pool =====

// BufferPool manages a pool of reusable byte buffers
type BufferPool struct {
	pool     sync.Pool
	size     int
	maxSize  int
	created  int64
	reused   int64
	returned int64
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		size:    size,
		maxSize: size * 2, // Allow buffers to grow up to 2x initial size
		pool: sync.Pool{
			New: func() interface{} {
				atomic.AddInt64(&bp.created, 1)
				return make([]byte, 0, size)
			},
		},
	}
}

var bp *BufferPool // For the pool.New closure

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() []byte {
	bp = p // Set for closure
	buf := p.pool.Get().([]byte)
	atomic.AddInt64(&p.reused, 1)
	return buf[:0] // Reset length but keep capacity
}

// GetWithSize retrieves a buffer with at least the specified capacity
func (p *BufferPool) GetWithSize(size int) []byte {
	buf := p.Get()
	if cap(buf) < size {
		// Buffer too small, allocate a new one
		atomic.AddInt64(&p.created, 1)
		return make([]byte, 0, size)
	}
	return buf
}

// Put returns a buffer to the pool
func (p *BufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}
	
	// Don't pool buffers that grew too large
	if cap(buf) > p.maxSize {
		return
	}
	
	atomic.AddInt64(&p.returned, 1)
	p.pool.Put(buf)
}

// Stats returns pool statistics
func (p *BufferPool) Stats() map[string]int64 {
	return map[string]int64{
		"created":  atomic.LoadInt64(&p.created),
		"reused":   atomic.LoadInt64(&p.reused),
		"returned": atomic.LoadInt64(&p.returned),
	}
}

// Global buffer pools for common sizes
var (
	SmallBufferPool  = NewBufferPool(4 * 1024)       // 4KB
	MediumBufferPool = NewBufferPool(64 * 1024)      // 64KB
	LargeBufferPool  = NewBufferPool(1024 * 1024)    // 1MB
	XLargeBufferPool = NewBufferPool(10 * 1024 * 1024) // 10MB
)

// GetPooledBuffer returns a buffer from the appropriate pool based on size
func GetPooledBuffer(size int) []byte {
	switch {
	case size <= 4*1024:
		return SmallBufferPool.GetWithSize(size)
	case size <= 64*1024:
		return MediumBufferPool.GetWithSize(size)
	case size <= 1024*1024:
		return LargeBufferPool.GetWithSize(size)
	default:
		return XLargeBufferPool.GetWithSize(size)
	}
}

// ReturnPooledBuffer returns a buffer to the appropriate pool
func ReturnPooledBuffer(buf []byte) {
	if buf == nil {
		return
	}
	
	capacity := cap(buf)
	switch {
	case capacity <= 4*1024*2: // Allow 2x growth
		SmallBufferPool.Put(buf)
	case capacity <= 64*1024*2:
		MediumBufferPool.Put(buf)
	case capacity <= 1024*1024*2:
		LargeBufferPool.Put(buf)
	case capacity <= 10*1024*1024*2:
		XLargeBufferPool.Put(buf)
	// Don't pool very large buffers
	}
}

// ===== Managed Buffer =====

// ManagedBuffer is a buffer that automatically returns to a pool when released
type ManagedBuffer struct {
	data     []byte
	pool     *BufferPool
	released int32 // atomic flag
}

// NewManagedBuffer creates a new managed buffer
func NewManagedBuffer(size int) *ManagedBuffer {
	pool := GetAppropriatePool(size)
	return &ManagedBuffer{
		data: pool.GetWithSize(size),
		pool: pool,
	}
}

// GetAppropriatePool returns the appropriate pool for a given size
func GetAppropriatePool(size int) *BufferPool {
	switch {
	case size <= 4*1024:
		return SmallBufferPool
	case size <= 64*1024:
		return MediumBufferPool
	case size <= 1024*1024:
		return LargeBufferPool
	default:
		return XLargeBufferPool
	}
}

// Data returns the underlying byte slice
func (mb *ManagedBuffer) Data() []byte {
	if atomic.LoadInt32(&mb.released) == 1 {
		return nil // Already released
	}
	return mb.data
}

// Resize resizes the buffer
func (mb *ManagedBuffer) Resize(newSize int) {
	if atomic.LoadInt32(&mb.released) == 1 {
		return // Already released
	}
	
	if newSize <= cap(mb.data) {
		mb.data = mb.data[:newSize]
		return
	}
	
	// Need a larger buffer
	oldData := mb.data
	newPool := GetAppropriatePool(newSize)
	mb.data = newPool.GetWithSize(newSize)
	copy(mb.data, oldData)
	
	// Return old buffer to its pool
	if mb.pool != nil {
		mb.pool.Put(oldData)
	}
	mb.pool = newPool
}

// Release returns the buffer to the pool
func (mb *ManagedBuffer) Release() {
	if atomic.CompareAndSwapInt32(&mb.released, 0, 1) {
		if mb.pool != nil && mb.data != nil {
			mb.pool.Put(mb.data)
			mb.data = nil
		}
	}
}

// SetFinalizer sets a finalizer to auto-release the buffer
func (mb *ManagedBuffer) SetFinalizer() {
	runtime.SetFinalizer(mb, (*ManagedBuffer).Release)
}

// ===== Thread Pool =====

// Task represents a compression/decompression task
type Task struct {
	Fn     func() error
	Result chan error
}

// ThreadPool manages a pool of worker threads
type ThreadPool struct {
	workers    int
	tasks      chan Task
	wg         sync.WaitGroup
	shutdown   chan struct{}
	shutdownMu sync.Mutex
	closed     bool
}

// NewThreadPool creates a new thread pool
func NewThreadPool(workers int) *ThreadPool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	
	tp := &ThreadPool{
		workers:  workers,
		tasks:    make(chan Task, workers*2),
		shutdown: make(chan struct{}),
	}
	
	// Start workers
	for i := 0; i < workers; i++ {
		tp.wg.Add(1)
		go tp.worker()
	}
	
	return tp
}

func (tp *ThreadPool) worker() {
	defer tp.wg.Done()
	
	for {
		select {
		case task := <-tp.tasks:
			err := task.Fn()
			if task.Result != nil {
				task.Result <- err
			}
		case <-tp.shutdown:
			return
		}
	}
}

// Submit submits a task to the pool
func (tp *ThreadPool) Submit(fn func() error) <-chan error {
	tp.shutdownMu.Lock()
	if tp.closed {
		tp.shutdownMu.Unlock()
		result := make(chan error, 1)
		result <- ErrPoolClosed
		return result
	}
	tp.shutdownMu.Unlock()
	
	result := make(chan error, 1)
	tp.tasks <- Task{
		Fn:     fn,
		Result: result,
	}
	
	return result
}

// SubmitNoWait submits a task without waiting for result
func (tp *ThreadPool) SubmitNoWait(fn func() error) {
	tp.shutdownMu.Lock()
	if tp.closed {
		tp.shutdownMu.Unlock()
		return
	}
	tp.shutdownMu.Unlock()
	
	select {
	case tp.tasks <- Task{Fn: fn}:
		// Task submitted
	default:
		// Queue full, execute synchronously
		fn()
	}
}

// Shutdown gracefully shuts down the pool
func (tp *ThreadPool) Shutdown() {
	tp.shutdownMu.Lock()
	if tp.closed {
		tp.shutdownMu.Unlock()
		return
	}
	tp.closed = true
	tp.shutdownMu.Unlock()
	
	close(tp.shutdown)
	tp.wg.Wait()
	close(tp.tasks)
}

// GlobalThreadPool is the global thread pool for parallel operations
var GlobalThreadPool = NewThreadPool(runtime.NumCPU())

// CompressParallel compresses data using the thread pool
func CompressParallel(dst, src []byte) ([]byte, error) {
	result := GlobalThreadPool.Submit(func() error {
		compressed := Compress(dst, src)
		dst = compressed
		return nil
	})
	
	err := <-result
	if err != nil {
		return nil, err
	}
	
	return dst, nil
}

// DecompressParallel decompresses data using the thread pool
func DecompressParallel(dst, src []byte) ([]byte, error) {
	resultChan := GlobalThreadPool.Submit(func() error {
		decompressed, err := Decompress(dst, src)
		dst = decompressed
		return err
	})
	
	err := <-resultChan
	if err != nil {
		return nil, err
	}
	
	return dst, nil
}
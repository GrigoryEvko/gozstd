package gozstd

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ThreadSafeWriter wraps Writer with thread safety
type ThreadSafeWriter struct {
	writer *Writer
	mu     sync.Mutex
	closed int32 // atomic
}

// NewThreadSafeWriter creates a thread-safe writer
func NewThreadSafeWriter(w io.Writer) *ThreadSafeWriter {
	return &ThreadSafeWriter{
		writer: NewWriter(w),
	}
}

// NewThreadSafeWriterLevel creates a thread-safe writer with compression level
func NewThreadSafeWriterLevel(w io.Writer, level int) *ThreadSafeWriter {
	return &ThreadSafeWriter{
		writer: NewWriterLevel(w, level),
	}
}

// Write implements io.Writer with thread safety
func (tsw *ThreadSafeWriter) Write(p []byte) (int, error) {
	if atomic.LoadInt32(&tsw.closed) != 0 {
		return 0, errors.New("writer is closed")
	}
	
	tsw.mu.Lock()
	defer tsw.mu.Unlock()
	
	return tsw.writer.Write(p)
}

// Flush flushes pending data
func (tsw *ThreadSafeWriter) Flush() error {
	if atomic.LoadInt32(&tsw.closed) != 0 {
		return errors.New("writer is closed")
	}
	
	tsw.mu.Lock()
	defer tsw.mu.Unlock()
	
	return tsw.writer.Flush()
}

// Close closes the writer
func (tsw *ThreadSafeWriter) Close() error {
	if !atomic.CompareAndSwapInt32(&tsw.closed, 0, 1) {
		return errors.New("writer already closed")
	}
	
	tsw.mu.Lock()
	defer tsw.mu.Unlock()
	
	err := tsw.writer.Close()
	tsw.writer.Release()
	return err
}

// ThreadSafeReader wraps Reader with thread safety
type ThreadSafeReader struct {
	reader *Reader
	mu     sync.Mutex
	closed int32 // atomic
}

// NewThreadSafeReader creates a thread-safe reader
func NewThreadSafeReader(r io.Reader) *ThreadSafeReader {
	return &ThreadSafeReader{
		reader: NewReader(r),
	}
}

// Read implements io.Reader with thread safety
func (tsr *ThreadSafeReader) Read(p []byte) (int, error) {
	if atomic.LoadInt32(&tsr.closed) != 0 {
		return 0, errors.New("reader is closed")
	}
	
	tsr.mu.Lock()
	defer tsr.mu.Unlock()
	
	return tsr.reader.Read(p)
}

// Close closes the reader
func (tsr *ThreadSafeReader) Close() error {
	if !atomic.CompareAndSwapInt32(&tsr.closed, 0, 1) {
		return errors.New("reader already closed")
	}
	
	tsr.mu.Lock()
	defer tsr.mu.Unlock()
	
	tsr.reader.Release()
	return nil
}

// SafeDictionary wraps dictionary with reference counting and thread safety
type SafeDictionary struct {
	cdict    *CDict
	ddict    *DDict
	refCount int32
	mu       sync.RWMutex
}

// NewSafeDictionary creates a safe dictionary from training data
func NewSafeDictionary(samples [][]byte, dictSize int) (*SafeDictionary, error) {
	dict := BuildDict(samples, dictSize)
	if len(dict) == 0 {
		return nil, errors.New("failed to build dictionary")
	}
	
	cdict, err := NewCDict(dict)
	if err != nil {
		return nil, fmt.Errorf("failed to create compression dictionary: %w", err)
	}
	
	ddict, err := NewDDict(dict)
	if err != nil {
		cdict.Release()
		return nil, fmt.Errorf("failed to create decompression dictionary: %w", err)
	}
	
	return &SafeDictionary{
		cdict:    cdict,
		ddict:    ddict,
		refCount: 1,
	}, nil
}

// AddRef increments reference count
func (sd *SafeDictionary) AddRef() {
	atomic.AddInt32(&sd.refCount, 1)
}

// Release decrements reference count and releases resources when zero
func (sd *SafeDictionary) Release() {
	if atomic.AddInt32(&sd.refCount, -1) == 0 {
		sd.mu.Lock()
		defer sd.mu.Unlock()
		
		if sd.cdict != nil {
			sd.cdict.Release()
			sd.cdict = nil
		}
		if sd.ddict != nil {
			sd.ddict.Release()
			sd.ddict = nil
		}
	}
}

// CompressDict compresses with the dictionary
func (sd *SafeDictionary) CompressDict(dst, src []byte) ([]byte, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	if sd.cdict == nil {
		return nil, errors.New("dictionary has been released")
	}
	
	return CompressDict(dst, src, sd.cdict), nil
}

// DecompressDict decompresses with the dictionary
func (sd *SafeDictionary) DecompressDict(dst, src []byte) ([]byte, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	if sd.ddict == nil {
		return nil, errors.New("dictionary has been released")
	}
	
	return DecompressDict(dst, src, sd.ddict)
}

// ParallelCompressor provides safe parallel compression
type ParallelCompressor struct {
	workers   int
	taskQueue chan *compressionTask
	wg        sync.WaitGroup
	closed    int32
}

type compressionTask struct {
	id       int
	data     []byte
	level    int
	result   []byte
	err      error
	done     chan struct{}
}

// NewParallelCompressor creates a parallel compressor
func NewParallelCompressor(workers int) *ParallelCompressor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	
	pc := &ParallelCompressor{
		workers:   workers,
		taskQueue: make(chan *compressionTask, workers*2),
	}
	
	// Start workers
	for i := 0; i < workers; i++ {
		pc.wg.Add(1)
		go pc.worker()
	}
	
	return pc
}

// worker processes compression tasks
func (pc *ParallelCompressor) worker() {
	defer pc.wg.Done()
	
	for task := range pc.taskQueue {
		task.result = CompressLevel(nil, task.data, task.level)
		close(task.done)
	}
}

// Compress compresses data in parallel
func (pc *ParallelCompressor) Compress(data []byte) ([]byte, error) {
	return pc.CompressLevel(data, DefaultCompressionLevel)
}

// CompressLevel compresses with specified level
func (pc *ParallelCompressor) CompressLevel(data []byte, level int) ([]byte, error) {
	if atomic.LoadInt32(&pc.closed) != 0 {
		return nil, errors.New("compressor is closed")
	}
	
	task := &compressionTask{
		data:  data,
		level: level,
		done:  make(chan struct{}),
	}
	
	select {
	case pc.taskQueue <- task:
		<-task.done
		return task.result, task.err
	case <-time.After(30 * time.Second):
		return nil, errors.New("compression timeout")
	}
}

// CompressBatch compresses multiple items in parallel
func (pc *ParallelCompressor) CompressBatch(items [][]byte) ([][]byte, error) {
	if atomic.LoadInt32(&pc.closed) != 0 {
		return nil, errors.New("compressor is closed")
	}
	
	tasks := make([]*compressionTask, len(items))
	for i, item := range items {
		tasks[i] = &compressionTask{
			id:    i,
			data:  item,
			level: DefaultCompressionLevel,
			done:  make(chan struct{}),
		}
		
		select {
		case pc.taskQueue <- tasks[i]:
			// Task queued
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("timeout queueing task %d", i)
		}
	}
	
	// Wait for all tasks
	results := make([][]byte, len(items))
	for i, task := range tasks {
		<-task.done
		if task.err != nil {
			return nil, fmt.Errorf("failed to compress item %d: %w", i, task.err)
		}
		results[i] = task.result
	}
	
	return results, nil
}

// Close closes the parallel compressor
func (pc *ParallelCompressor) Close() error {
	if !atomic.CompareAndSwapInt32(&pc.closed, 0, 1) {
		return errors.New("compressor already closed")
	}
	
	close(pc.taskQueue)
	pc.wg.Wait()
	return nil
}

// SafeContextPool manages compression contexts safely
type SafeContextPool struct {
	pool     sync.Pool
	maxItems int32
	current  int32
}

// NewSafeContextPool creates a safe context pool
func NewSafeContextPool(maxItems int) *SafeContextPool {
	return &SafeContextPool{
		maxItems: int32(maxItems),
		pool: sync.Pool{
			New: func() interface{} {
				return NewCCtx()
			},
		},
	}
}

// Get gets a context from the pool
func (scp *SafeContextPool) Get() *CCtx {
	if atomic.LoadInt32(&scp.current) < scp.maxItems {
		if ctx := scp.pool.Get(); ctx != nil {
			atomic.AddInt32(&scp.current, 1)
			return ctx.(*CCtx)
		}
	}
	
	// Pool is full or empty, create new
	return NewCCtx()
}

// Put returns a context to the pool
func (scp *SafeContextPool) Put(ctx *CCtx) {
	if ctx == nil {
		return
	}
	
	// Reset context before returning to pool
	ctx.Reset(ZSTD_reset_session_and_parameters)
	
	if atomic.LoadInt32(&scp.current) < scp.maxItems {
		scp.pool.Put(ctx)
	} else {
		// Pool is full, release the context
		ctx.Release()
		atomic.AddInt32(&scp.current, -1)
	}
}

// ConcurrentMap provides thread-safe map for caching
type ConcurrentMap struct {
	shards []*mapShard
	count  int
}

type mapShard struct {
	mu    sync.RWMutex
	items map[string][]byte
}

// NewConcurrentMap creates a concurrent map with specified shard count
func NewConcurrentMap(shardCount int) *ConcurrentMap {
	if shardCount <= 0 {
		shardCount = 32
	}
	
	cm := &ConcurrentMap{
		shards: make([]*mapShard, shardCount),
		count:  shardCount,
	}
	
	for i := range cm.shards {
		cm.shards[i] = &mapShard{
			items: make(map[string][]byte),
		}
	}
	
	return cm
}

// getShard returns the shard for a key
func (cm *ConcurrentMap) getShard(key string) *mapShard {
	hash := 0
	for _, c := range key {
		hash = hash*31 + int(c)
	}
	return cm.shards[uint(hash)%uint(cm.count)]
}

// Set sets a value
func (cm *ConcurrentMap) Set(key string, value []byte) {
	shard := cm.getShard(key)
	shard.mu.Lock()
	shard.items[key] = value
	shard.mu.Unlock()
}

// Get gets a value
func (cm *ConcurrentMap) Get(key string) ([]byte, bool) {
	shard := cm.getShard(key)
	shard.mu.RLock()
	value, ok := shard.items[key]
	shard.mu.RUnlock()
	return value, ok
}

// Delete deletes a value
func (cm *ConcurrentMap) Delete(key string) {
	shard := cm.getShard(key)
	shard.mu.Lock()
	delete(shard.items, key)
	shard.mu.Unlock()
}

// Clear clears all values
func (cm *ConcurrentMap) Clear() {
	for _, shard := range cm.shards {
		shard.mu.Lock()
		shard.items = make(map[string][]byte)
		shard.mu.Unlock()
	}
}

// Size returns the total number of items
func (cm *ConcurrentMap) Size() int {
	size := 0
	for _, shard := range cm.shards {
		shard.mu.RLock()
		size += len(shard.items)
		shard.mu.RUnlock()
	}
	return size
}
package gozstd

import (
	"sync"
	"sync/atomic"
)

// BufferPool manages reusable byte slices with size-based pools
type BufferPool struct {
	pools    []*sync.Pool // Size classes: 1KB, 2KB, 4KB, 8KB, 16KB, 32KB, 64KB, 128KB, 256KB, 512KB
	maxItems int64        // Maximum items per pool to prevent unbounded growth
	counts   []int64      // Current count per pool (atomic)
}

// Global buffer pool instance
var globalBufferPool = &BufferPool{
	pools:    make([]*sync.Pool, 10),
	maxItems: 50, // Limit each size class to 50 items
	counts:   make([]int64, 10),
}

func init() {
	// Initialize pools for different size classes
	for i := range globalBufferPool.pools {
		size := 1024 << i // 1KB, 2KB, 4KB, 8KB, 16KB, 32KB, 64KB, 128KB, 256KB, 512KB
		globalBufferPool.pools[i] = &sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, size)
			},
		}
	}
}

// GetBuffer returns a buffer with at least minCapacity.
// The returned buffer may have zero length but guaranteed capacity.
func GetBuffer(minCapacity int) []byte {
	if minCapacity <= 0 {
		return nil
	}
	
	// Find the appropriate size class
	for i, pool := range globalBufferPool.pools {
		poolSize := 1024 << i
		if poolSize >= minCapacity {
			if buf := pool.Get(); buf != nil {
				b := buf.([]byte)
				return b[:0] // Reset length to 0, keep capacity
			}
			// Pool was empty, create new buffer
			return make([]byte, 0, poolSize)
		}
	}
	
	// For very large buffers (>512KB), allocate directly
	return make([]byte, 0, minCapacity)
}

// PutBuffer returns a buffer to the pool for reuse.
// The buffer should not be used after calling PutBuffer.
func PutBuffer(buf []byte) {
	if buf == nil {
		return
	}
	
	capacity := cap(buf)
	
	// Find matching pool by capacity
	for i, pool := range globalBufferPool.pools {
		if pool == nil {
			continue // Skip nil pools
		}
		
		poolSize := 1024 << i
		if poolSize == capacity {
			// Check if pool is not too full
			if atomic.LoadInt64(&globalBufferPool.counts[i]) < globalBufferPool.maxItems {
				// Reset to zero length (no need to clear data - new users will overwrite)
				buf = buf[:0]
				
				pool.Put(buf)
				atomic.AddInt64(&globalBufferPool.counts[i], 1)
			}
			return
		}
	}
	
	// Non-standard size, let GC handle it
}

// GetCompressBuffer returns a buffer suitable for compression output.
// It estimates the required size based on input size and compression bound.
func GetCompressBuffer(inputSize int) []byte {
	// ZSTD compression bound with some headroom
	estimatedSize := inputSize + (inputSize >> 8) + 64
	return GetBuffer(estimatedSize)
}

// GetDecompressBuffer returns a buffer suitable for decompression output.
// It uses the decompressed size hint when available.
func GetDecompressBuffer(hint int) []byte {
	if hint <= 0 {
		// Default size for unknown decompression size
		hint = 64 * 1024 // 64KB default
	}
	return GetBuffer(hint)
}

// OptimizeBuffer reduces buffer capacity if there's significant waste.
// Returns the same buffer if optimization isn't beneficial.
func OptimizeBuffer(buf []byte) []byte {
	if buf == nil {
		return nil
	}
	
	length := len(buf)
	capacity := cap(buf)
	
	// Only optimize if waste is significant (>50% of capacity unused)
	if capacity > length*2 && length > 0 {
		// Get a more appropriately sized buffer
		newBuf := GetBuffer(length)
		copy(newBuf[:cap(newBuf)], buf)
		
		// Return old buffer to pool
		PutBuffer(buf[:0])
		
		return newBuf[:length]
	}
	
	return buf
}

// BufferPoolStats returns statistics about buffer pool usage
type BufferPoolStats struct {
	PoolCounts    []int64 // Items in each pool
	TotalBuffers  int64   // Total buffers across all pools
	PoolSizes     []int   // Size of each pool in bytes
}

// GetStats returns current buffer pool statistics
func GetBufferPoolStats() BufferPoolStats {
	stats := BufferPoolStats{
		PoolCounts: make([]int64, len(globalBufferPool.pools)),
		PoolSizes:  make([]int, len(globalBufferPool.pools)),
	}
	
	for i := range globalBufferPool.pools {
		stats.PoolCounts[i] = atomic.LoadInt64(&globalBufferPool.counts[i])
		stats.TotalBuffers += stats.PoolCounts[i]
		stats.PoolSizes[i] = 1024 << i
	}
	
	return stats
}

// ClearBufferPools clears all buffer pools (useful for testing)
func ClearBufferPools() {
	for i := range globalBufferPool.pools {
		// Reset the pool by creating a new one
		poolSize := 1024 << i
		globalBufferPool.pools[i] = &sync.Pool{
			New: func(size int) func() interface{} {
				return func() interface{} {
					return make([]byte, 0, size)
				}
			}(poolSize),
		}
		atomic.StoreInt64(&globalBufferPool.counts[i], 0)
	}
}
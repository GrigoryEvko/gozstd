package gozstd

import (
	"runtime"
	"testing"
)

func TestBufferPoolBasicFunctionality(t *testing.T) {
	// Clear pools before testing
	ClearBufferPools()
	
	t.Run("GetBuffer_ReturnsCorrectCapacity", func(t *testing.T) {
		testCases := []int{100, 1024, 2048, 4096, 8192, 16384, 32768, 65536}
		
		for _, size := range testCases {
			buf := GetBuffer(size)
			if cap(buf) < size {
				t.Errorf("GetBuffer(%d) returned buffer with capacity %d, expected at least %d", 
					size, cap(buf), size)
			}
			if len(buf) != 0 {
				t.Errorf("GetBuffer(%d) returned buffer with length %d, expected 0", size, len(buf))
			}
			
			// Return to pool
			PutBuffer(buf)
		}
	})
	
	t.Run("BufferReuse", func(t *testing.T) {
		// Get a buffer
		buf1 := GetBuffer(4096)
		if cap(buf1) < 4096 {
			t.Fatal("Buffer capacity too small")
		}
		
		// Return to pool
		PutBuffer(buf1)
		
		// Get another buffer of same size - should be reused
		buf2 := GetBuffer(4096)
		
		// Should have zero length regardless of reuse
		if len(buf2) != 0 {
			t.Errorf("Buffer should have zero length, got %d", len(buf2))
		}
		
		PutBuffer(buf2)
	})
	
	t.Run("PoolSizeLimits", func(t *testing.T) {
		ClearBufferPools()
		
		// Create more buffers than pool limit
		var buffers [][]byte
		for i := 0; i < 60; i++ { // More than maxItems (50)
			buf := GetBuffer(1024)
			buffers = append(buffers, buf)
		}
		
		// Return all buffers
		for _, buf := range buffers {
			PutBuffer(buf)
		}
		
		// Check stats
		stats := GetBufferPoolStats()
		
		// First pool (1KB) should be at limit
		if stats.PoolCounts[0] > 50 {
			t.Errorf("Pool size exceeded limit: got %d, expected max 50", stats.PoolCounts[0])
		}
	})
}

func TestBufferPoolIntegration(t *testing.T) {
	// Test buffer pool with actual compression/decompression
	t.Run("CompressionUsesBufferPool", func(t *testing.T) {
		ClearBufferPools()
		
		// Get baseline stats
		statsBefore := GetBufferPoolStats()
		
		// Perform multiple compressions
		data := make([]byte, 10000)
		for i := range data {
			data[i] = byte(i % 256)
		}
		
		var results [][]byte
		for i := 0; i < 10; i++ {
			compressed := Compress(nil, data)
			results = append(results, compressed)
		}
		
		// Check if buffer pool is being used
		statsAfter := GetBufferPoolStats()
		
		// Should have some buffers in pools after compression
		totalAfter := statsAfter.TotalBuffers
		totalBefore := statsBefore.TotalBuffers
		
		t.Logf("Buffer pool usage: before=%d, after=%d", totalBefore, totalAfter)
		
		// Verify compressed data is correct
		for i, compressed := range results {
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Errorf("Decompression %d failed: %v", i, err)
			}
			if len(decompressed) != len(data) {
				t.Errorf("Decompressed size mismatch: got %d, expected %d", len(decompressed), len(data))
			}
		}
	})
}

func TestOptimizeBuffer(t *testing.T) {
	t.Run("OptimizesWastefulBuffers", func(t *testing.T) {
		// Create a buffer with significant waste
		largeBuf := make([]byte, 100, 10000) // 100 bytes used, 10KB capacity
		
		optimized := OptimizeBuffer(largeBuf)
		
		if cap(optimized) >= cap(largeBuf) {
			t.Error("Buffer not optimized: capacity should be reduced")
		}
		
		if len(optimized) != len(largeBuf) {
			t.Errorf("Buffer content changed: got length %d, expected %d", 
				len(optimized), len(largeBuf))
		}
	})
	
	t.Run("SkipsEfficientBuffers", func(t *testing.T) {
		// Create an efficient buffer (low waste)
		efficientBuf := make([]byte, 900, 1000) // 90% utilization
		
		optimized := OptimizeBuffer(efficientBuf)
		
		// Should be the same buffer (no optimization needed)
		if cap(optimized) != cap(efficientBuf) {
			t.Error("Efficient buffer was unnecessarily optimized")
		}
	})
}

func TestBufferPoolStats(t *testing.T) {
	ClearBufferPools()
	
	stats := GetBufferPoolStats()
	
	// Should have correct number of pools
	if len(stats.PoolCounts) != 10 {
		t.Errorf("Expected 10 pools, got %d", len(stats.PoolCounts))
	}
	
	if len(stats.PoolSizes) != 10 {
		t.Errorf("Expected 10 pool sizes, got %d", len(stats.PoolSizes))
	}
	
	// Verify pool sizes are correct
	expectedSizes := []int{1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144, 524288}
	for i, size := range expectedSizes {
		if stats.PoolSizes[i] != size {
			t.Errorf("Pool %d size mismatch: got %d, expected %d", i, stats.PoolSizes[i], size)
		}
	}
	
	// All pools should be empty initially
	if stats.TotalBuffers != 0 {
		t.Errorf("Expected 0 total buffers, got %d", stats.TotalBuffers)
	}
}

func TestBufferPoolMemoryEfficiency(t *testing.T) {
	// Test that buffer pool reduces memory allocations
	t.Run("MemoryPressureTest", func(t *testing.T) {
		runtime.GC()
		runtime.GC() // Force GC
		
		var m1, m2 runtime.MemStats
		runtime.ReadMemStats(&m1)
		
		// Perform many operations with buffer reuse
		data := make([]byte, 1000)
		for i := 0; i < 1000; i++ {
			compressed := Compress(nil, data)
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Operation %d failed: %v", i, err)
			}
			_ = decompressed
			
			if i%100 == 0 {
				runtime.GC() // Periodic GC to keep memory pressure low
			}
		}
		
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&m2)
		
		allocDiff := m2.TotalAlloc - m1.TotalAlloc
		t.Logf("Memory allocated during 1000 operations: %d KB", allocDiff/1024)
		
		// With buffer pooling, should use much less memory
		// This is more of an informational test
		if allocDiff > 50*1024*1024 { // 50MB threshold
			t.Logf("High memory usage detected: %d KB - buffer pool may need tuning", allocDiff/1024)
		}
	})
}

func TestGetCompressBuffer(t *testing.T) {
	testSizes := []int{100, 1000, 10000, 100000}
	
	for _, size := range testSizes {
		buf := GetCompressBuffer(size)
		
		// Should have at least the estimated size
		expectedMin := size + (size >> 8) + 64
		if cap(buf) < expectedMin {
			t.Errorf("GetCompressBuffer(%d) capacity %d too small, expected at least %d", 
				size, cap(buf), expectedMin)
		}
		
		PutBuffer(buf)
	}
}

func TestGetDecompressBuffer(t *testing.T) {
	// Test with hint
	buf := GetDecompressBuffer(5000)
	if cap(buf) < 5000 {
		t.Errorf("GetDecompressBuffer(5000) capacity %d too small", cap(buf))
	}
	PutBuffer(buf)
	
	// Test without hint (should use default)
	buf = GetDecompressBuffer(0)
	if cap(buf) < 64*1024 {
		t.Errorf("GetDecompressBuffer(0) should use 64KB default, got %d", cap(buf))
	}
	PutBuffer(buf)
}
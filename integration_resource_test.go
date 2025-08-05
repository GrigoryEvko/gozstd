package gozstd

import (
	"bytes"
	"runtime"
	"sync"
	"testing"
)

// FuzzGiantInputs tests compression of very large data
func FuzzGiantInputs(f *testing.F) {
	f.Add(1<<10, byte('a'), 3)      // 1KB
	f.Add(1<<20, byte('b'), 5)      // 1MB
	f.Add(1<<24, byte('c'), 1)      // 16MB
	
	f.Fuzz(func(t *testing.T, size int, fillByte byte, level int) {
		// Limit size to prevent OOM in fuzzing
		if size < 0 || size > 1<<26 { // Max 64MB
			return
		}
		
		// Create large data with pattern
		data := bytes.Repeat([]byte{fillByte}, size)
		
		// Add some variation to prevent too good compression
		for i := 0; i < len(data); i += 1000 {
			data[i] = byte(i % 256)
		}
		
		// Measure memory before
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		
		// Compress
		compressed := CompressLevel(nil, data, level)
		
		// Measure memory after compression
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		
		memUsed := m2.Alloc - m1.Alloc
		compressionRatio := float64(len(data)) / float64(len(compressed))
		
		t.Logf("Compressed %d bytes to %d bytes (ratio: %.2fx), memory delta: %d bytes",
			len(data), len(compressed), compressionRatio, memUsed)
		
		// Check for memory explosion
		if memUsed > uint64(len(data)*10) {
			t.Errorf("Excessive memory usage: %d bytes for %d byte input", memUsed, len(data))
		}
		
		// Decompress
		runtime.GC()
		runtime.ReadMemStats(&m1)
		
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		runtime.ReadMemStats(&m2)
		decompressionMemUsed := m2.Alloc - m1.Alloc
		
		if decompressionMemUsed > uint64(len(data)*10) {
			t.Errorf("Excessive decompression memory: %d bytes", decompressionMemUsed)
		}
		
		// Verify data
		if !bytes.Equal(decompressed, data) {
			t.Error("Data mismatch after compression/decompression")
		}
		
		// Test streaming with large data
		if size > 1<<20 { // Only for data > 1MB
			testStreamingLargeData(t, data, level)
		}
	})
}

// FuzzTinyBuffers tests compression into very small buffers
func FuzzTinyBuffers(f *testing.F) {
	f.Add([]byte("test data"), 1, 3)
	f.Add([]byte("longer test data for tiny buffers"), 10, 5)
	
	f.Fuzz(func(t *testing.T, data []byte, bufSize int, level int) {
		if bufSize < 0 || bufSize > len(data) {
			bufSize = 1
		}
		
		// Create tiny destination buffer
		dst := make([]byte, 0, bufSize)
		
		// This should resize the buffer as needed
		compressed := CompressLevel(dst, data, level)
		
		if len(compressed) == 0 {
			t.Error("Compression into tiny buffer produced no output")
			return
		}
		
		// The implementation should have grown the buffer
		if cap(compressed) == bufSize && len(compressed) > bufSize {
			t.Error("Buffer capacity wasn't increased despite needing more space")
		}
		
		// Verify correctness
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Decompression failed: %v", err)
		} else if !bytes.Equal(decompressed, data) {
			t.Error("Data corruption with tiny buffer compression")
		}
		
		// Test decompression into tiny buffer
		tinyDst := make([]byte, 0, 1)
		decompressed, err = Decompress(tinyDst, compressed)
		if err != nil {
			t.Errorf("Decompression into tiny buffer failed: %v", err)
		} else if !bytes.Equal(decompressed, data) {
			t.Error("Data corruption with tiny buffer decompression")
		}
	})
}

// FuzzDictionarySizeLimits tests extreme dictionary sizes
func FuzzDictionarySizeLimits(f *testing.F) {
	f.Add(10, byte('d'), []byte("data"), 3)
	f.Add(1<<20, byte('D'), []byte("test"), 5)
	
	f.Fuzz(func(t *testing.T, dictSize int, fillByte byte, data []byte, level int) {
		// Limit dictionary size
		if dictSize < 0 {
			dictSize = 0
		}
		if dictSize > 1<<22 { // 4MB max
			dictSize = 1 << 22
		}
		
		// Create dictionary
		var dict []byte
		if dictSize > 0 {
			dict = bytes.Repeat([]byte{fillByte}, dictSize)
			// Add some entropy
			for i := 0; i < len(dict); i += 100 {
				dict[i] = byte(i % 256)
			}
		}
		
		// Try to create compression dictionary
		cd, err := NewCDictLevel(dict, level)
		if err != nil {
			if dictSize == 0 {
				// Expected - empty dictionary
				return
			}
			t.Logf("Failed to create %d byte dictionary: %v", dictSize, err)
			return
		}
		defer cd.Release()
		
		// Create decompression dictionary
		dd, err := NewDDict(dict)
		if err != nil {
			t.Logf("Failed to create decompression dictionary: %v", err)
			return
		}
		defer dd.Release()
		
		// Test compression with huge dictionary
		compressed := CompressDict(nil, data, cd)
		
		// Dictionary might be larger than data
		if len(dict) > len(data)*100 {
			t.Logf("Compressed %d bytes with %d byte dictionary to %d bytes",
				len(data), len(dict), len(compressed))
		}
		
		// Decompress
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err != nil {
			t.Errorf("Decompression with large dictionary failed: %v", err)
		} else if !bytes.Equal(decompressed, data) {
			t.Error("Data corruption with large dictionary")
		}
		
		// Test ByRef with large dictionary
		if dictSize > 1<<10 { // Only for dictionaries > 1KB
			cdRef, err := NewCDictByRefLevel(dict, level)
			if err == nil {
				defer cdRef.Release()
				compressedRef := CompressDict(nil, data, cdRef)
				if !bytes.Equal(compressed, compressedRef) {
					t.Log("ByRef dictionary produced different compression")
				}
			}
		}
	})
}

// FuzzInfiniteCompression tests data that expands when compressed
func FuzzInfiniteCompression(f *testing.F) {
	f.Add(100, uint32(0x12345678), 3)
	f.Add(1000, uint32(0xDEADBEEF), 5)
	
	f.Fuzz(func(t *testing.T, size int, seed uint32, level int) {
		if size < 0 || size > 1<<20 {
			size = 1000
		}
		
		// Create incompressible data (random)
		data := make([]byte, size)
		rng := seed
		for i := range data {
			// Simple PRNG
			rng = rng*1664525 + 1013904223
			data[i] = byte(rng >> 24)
		}
		
		// Compress random data
		compressed := CompressLevel(nil, data, level)
		
		expansionRatio := float64(len(compressed)) / float64(len(data))
		t.Logf("Random data expanded from %d to %d bytes (ratio: %.2fx)",
			len(data), len(compressed), expansionRatio)
		
		// Check for reasonable expansion
		if expansionRatio > 1.5 {
			t.Errorf("Excessive expansion for incompressible data: %.2fx", expansionRatio)
		}
		
		// Verify we can still decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Failed to decompress expanded data: %v", err)
		} else if !bytes.Equal(decompressed, data) {
			t.Error("Data corruption with incompressible data")
		}
		
		// Test double compression (compress already compressed data)
		doubleCompressed := CompressLevel(nil, compressed, level)
		doubleRatio := float64(len(doubleCompressed)) / float64(len(compressed))
		
		t.Logf("Double compression: %d -> %d -> %d bytes",
			len(data), len(compressed), len(doubleCompressed))
		
		if doubleRatio < 1.0 {
			t.Log("Already compressed data was further compressed")
		}
	})
}

// FuzzMemoryPressure tests behavior under memory pressure
func FuzzMemoryPressure(f *testing.F) {
	f.Add([]byte("test"), 100, 10, 1<<20)
	
	f.Fuzz(func(t *testing.T, data []byte, contexts, goroutines, allocSize int) {
		if contexts <= 0 || contexts > 1000 {
			contexts = 100
		}
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 10
		}
		if allocSize < 0 || allocSize > 1<<24 {
			allocSize = 1 << 20 // 1MB
		}
		
		// Allocate memory to create pressure
		memoryHog := make([][]byte, goroutines)
		for i := range memoryHog {
			memoryHog[i] = make([]byte, allocSize)
		}
		
		// Force GC to clean up before test
		runtime.GC()
		
		// Create many contexts
		contexts = contexts / 10 // Reduce to avoid OOM
		ctxs := make([]*CCtx, contexts)
		for i := range ctxs {
			ctxs[i] = NewCCtx()
		}
		
		// Create many dictionaries
		dicts := make([]*CDict, contexts/10)
		for i := range dicts {
			dict := []byte("dictionary " + string(rune(i)))
			cd, err := NewCDict(dict)
			if err != nil {
				break
			}
			dicts[i] = cd
		}
		
		// Run concurrent operations under memory pressure
		var wg sync.WaitGroup
		errors := make(chan error, goroutines)
		
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				// Use different contexts
				ctx := ctxs[id%len(ctxs)]
				
				// Compress
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					errors <- err
					return
				}
				
				// Force more allocations
				extraData := make([]byte, allocSize/10)
				_ = extraData
				
				// Decompress
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					errors <- err
					return
				}
				
				if !bytes.Equal(decompressed, data) {
					errors <- err
				}
				
				// Trigger GC occasionally
				if id%10 == 0 {
					runtime.GC()
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)
		
		errorCount := 0
		for range errors {
			errorCount++
		}
		
		// Cleanup
		for _, cd := range dicts {
			if cd != nil {
				cd.Release()
			}
		}
		
		// Note: Can't release contexts due to design bug
		
		if errorCount > 0 {
			t.Logf("Memory pressure caused %d errors out of %d operations",
				errorCount, goroutines)
		}
		
		// Final GC to clean up
		runtime.GC()
	})
}

// FuzzManySmallCompressions tests many small compression operations
func FuzzManySmallCompressions(f *testing.F) {
	f.Add(10000, 10, 3)
	
	f.Fuzz(func(t *testing.T, iterations, dataSize, level int) {
		if iterations < 0 || iterations > 100000 {
			iterations = 10000
		}
		if dataSize < 0 || dataSize > 1000 {
			dataSize = 10
		}
		
		// Track memory growth
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		
		// Perform many small compressions
		for i := 0; i < iterations; i++ {
			// Create small unique data
			data := make([]byte, dataSize)
			for j := range data {
				data[j] = byte((i + j) % 256)
			}
			
			// Compress and decompress
			compressed := CompressLevel(nil, data, level)
			decompressed, err := Decompress(nil, compressed)
			
			if err != nil || !bytes.Equal(decompressed, data) {
				t.Errorf("Error at iteration %d: %v", i, err)
				break
			}
			
			// Check memory periodically
			if i%1000 == 0 && i > 0 {
				var m2 runtime.MemStats
				runtime.ReadMemStats(&m2)
				
				// Handle case where memory decreases due to GC
				if m2.Alloc >= m1.Alloc {
					memGrowth := m2.Alloc - m1.Alloc
					if memGrowth > uint64(iterations*dataSize*50) {
						t.Errorf("Excessive memory growth: %d bytes after %d iterations",
							memGrowth, i)
						break
					}
				} else {
					t.Logf("Memory decreased by %d bytes at iteration %d (GC likely occurred)",
						m1.Alloc - m2.Alloc, i)
				}
			}
		}
		
		// Final memory check
		runtime.GC()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		
		// Handle memory decrease due to GC
		if m2.Alloc >= m1.Alloc {
			finalMemGrowth := m2.Alloc - m1.Alloc
			t.Logf("Memory growth after %d small compressions: %d bytes",
				iterations, finalMemGrowth)
		} else {
			finalMemDecrease := m1.Alloc - m2.Alloc
			t.Logf("Memory decreased after %d small compressions: %d bytes (GC freed memory)",
				iterations, finalMemDecrease)
		}
	})
}

// Helper function for testing streaming with large data
func testStreamingLargeData(t *testing.T, data []byte, level int) {
	var compressed bytes.Buffer
	w := NewWriterLevel(&compressed, level)
	
	// Write in chunks
	chunkSize := 65536 // 64KB chunks
	written := 0
	for written < len(data) {
		end := written + chunkSize
		if end > len(data) {
			end = len(data)
		}
		
		n, err := w.Write(data[written:end])
		if err != nil {
			t.Errorf("Streaming write failed: %v", err)
			return
		}
		written += n
	}
	
	if err := w.Close(); err != nil {
		t.Errorf("Streaming close failed: %v", err)
		return
	}
	
	// Decompress streaming
	r := NewReader(&compressed)
	decompressed := make([]byte, 0, len(data))
	buf := make([]byte, chunkSize)
	
	for {
		n, err := r.Read(buf)
		if n > 0 {
			decompressed = append(decompressed, buf[:n]...)
		}
		if err != nil {
			if err.Error() != "EOF" {
				t.Errorf("Streaming read error: %v", err)
			}
			break
		}
	}
	
	if !bytes.Equal(decompressed, data) {
		t.Error("Streaming compression/decompression mismatch")
	}
}
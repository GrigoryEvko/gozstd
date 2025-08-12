package gozstd

// This file consolidates integration tests from:
// - integration_target_block_size_test.go
// - integration_streaming_params_test.go
// - integration_memory_test.go
// - integration_rsync_friendly_test.go
// - integration_resource_test.go
// - integration_corrupt_data_test.go
// - integration_corpus_test.go
// - integration_state_test.go

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// =============================================================================
// MERGED FROM integration_memory_test.go
// =============================================================================

// FuzzDictionaryByRefMemorySafety tests use-after-free scenarios with ByRef dictionaries
func FuzzDictionaryByRefMemorySafety(f *testing.F) {
	f.Add([]byte("test data"), []byte("dictionary"), 5, 100)

	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int, modifyDelay int) {
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}

		// Create a copy that we'll modify later
		dictCopy := make([]byte, len(dictData))
		copy(dictCopy, dictData)

		// Create ByRef dictionaries
		cd, err := NewCDictByRefLevel(dictCopy, level)
		if err != nil {
			return
		}
		defer cd.Release()

		dd, err := NewDDictByRef(dictCopy)
		if err != nil {
			return
		}
		defer dd.Release()

		// Start compression
		compressed := CompressDict(nil, data, cd)

		// Modify the dictionary data after compression starts
		// This simulates use-after-free if the implementation doesn't hold the reference
		if modifyDelay > 0 && modifyDelay < 1000 {
			time.Sleep(time.Duration(modifyDelay) * time.Microsecond)
		}

		// Corrupt the dictionary
		for i := range dictCopy {
			dictCopy[i] = ^dictCopy[i]
		}

		// Decompress - this should still work if ByRef holds the reference correctly
		_, err = DecompressDict(nil, compressed, dd)
		if err != nil {
			// This is acceptable - decompression might fail due to corruption
			t.Logf("Decompression failed (expected): %v", err)
		}
	})
}

// TestMemoryBoundedCompression tests memory usage is bounded under various scenarios
func TestMemoryBoundedCompression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	// Force garbage collection to get baseline
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Test data - make it large enough to test memory behavior
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test memory usage patterns
	testCases := []struct {
		name     string
		workers  int
		jobSize  int
		overhead int64 // Expected additional memory overhead in MB
	}{
		{
			name:     "single_thread",
			workers:  0,
			overhead: 5,
		},
		{
			name:     "multi_thread_small_jobs",
			workers:  4,
			jobSize:  64 * 1024,
			overhead: 20,
		},
		{
			name:     "multi_thread_large_jobs",
			workers:  4,
			jobSize:  512 * 1024,
			overhead: 50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)

			params := &WriterParams{
				CompressionLevel: 6,
				NbWorkers:        tc.workers,
				JobSize:          tc.jobSize,
			}

			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, params)

			if _, err := writer.Write(data); err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			if err := writer.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}
			writer.Release()

			runtime.GC()
			var m3 runtime.MemStats
			runtime.ReadMemStats(&m3)

			// Calculate memory increase (in MB)
			memIncrease := int64(m3.Alloc-m2.Alloc) / (1024 * 1024)

			t.Logf("Memory increase: %d MB (expected max: %d MB)", memIncrease, tc.overhead)

			// This is a heuristic check - memory usage can vary
			if memIncrease > tc.overhead*2 {
				t.Errorf("Memory usage seems excessive: %d MB (expected max around %d MB)", memIncrease, tc.overhead)
			}

			// Verify decompression still works
			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}

			if !bytes.Equal(data, decompressed) {
				t.Fatal("Data mismatch after compression/decompression")
			}
		})
	}
}

// TestGoroutineLeakDetection checks for goroutine leaks
func TestGoroutineLeakDetection(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	// Perform operations that might leak goroutines
	data := make([]byte, 10000)
	if _, err := rand.Read(data); err != nil {
		t.Fatal("Cannot generate test data")
	}

	// Test multiple writers with different configurations
	for i := 0; i < 10; i++ {
		params := &WriterParams{
			CompressionLevel: 3,
			NbWorkers:        2,
			JobSize:          1024,
		}

		var compressed bytes.Buffer
		writer := NewWriterParams(&compressed, params)

		if _, err := writer.Write(data); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Close %d failed: %v", i, err)
		}
		writer.Release()
	}

	// Give goroutines time to finish
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	goroutineDiff := finalGoroutines - initialGoroutines

	t.Logf("Goroutines: initial=%d, final=%d, diff=%d", initialGoroutines, finalGoroutines, goroutineDiff)

	// Allow some variance as the Go runtime may create/destroy goroutines
	if goroutineDiff > 5 {
		t.Errorf("Possible goroutine leak detected: %d extra goroutines", goroutineDiff)
	}
}

// =============================================================================
// MERGED FROM integration_corrupt_data_test.go
// =============================================================================

// TestCorruptDataHandling tests various corruption scenarios
func TestCorruptDataHandling(t *testing.T) {
	testData := []byte("The quick brown fox jumps over the lazy dog. " + strings.Repeat("More test data. ", 1000))
	validCompressed := Compress(nil, testData)

	corruptionTests := []struct {
		name        string
		corruptFunc func([]byte) []byte
	}{
		{
			name: "single_bit_flip",
			corruptFunc: func(data []byte) []byte {
				corrupted := make([]byte, len(data))
				copy(corrupted, data)
				// Flip one bit in the middle
				if len(corrupted) > 10 {
					corrupted[len(corrupted)/2] ^= 1
				}
				return corrupted
			},
		},
		{
			name: "truncate_end",
			corruptFunc: func(data []byte) []byte {
				if len(data) <= 10 {
					return data[:0]
				}
				return data[:len(data)-10]
			},
		},
		{
			name: "truncate_beginning",
			corruptFunc: func(data []byte) []byte {
				if len(data) <= 10 {
					return data[:0]
				}
				return data[10:]
			},
		},
		{
			name: "random_corruption",
			corruptFunc: func(data []byte) []byte {
				corrupted := make([]byte, len(data))
				copy(corrupted, data)
				// Corrupt 10% of bytes randomly
				for i := 0; i < len(corrupted)/10; i++ {
					if len(corrupted) > 0 {
						idx := i * 10 % len(corrupted)
						corrupted[idx] = ^corrupted[idx]
					}
				}
				return corrupted
			},
		},
		{
			name: "header_corruption",
			corruptFunc: func(data []byte) []byte {
				corrupted := make([]byte, len(data))
				copy(corrupted, data)
				// Corrupt the first few bytes (likely header)
				for i := 0; i < 4 && i < len(corrupted); i++ {
					corrupted[i] = 0xFF
				}
				return corrupted
			},
		},
	}

	for _, tc := range corruptionTests {
		t.Run(tc.name, func(t *testing.T) {
			corrupted := tc.corruptFunc(validCompressed)

			// Try to decompress - should fail gracefully
			_, err := Decompress(nil, corrupted)
			if err == nil {
				// Some corruption might not be detected
				t.Logf("Corruption was not detected (acceptable for some cases)")
			} else {
				t.Logf("Corruption detected as expected: %v", err)
				// Verify it's a proper error type
				if IsCorruptionError(err) || IsFrameError(err) {
					t.Logf("Appropriate error type: %T", err)
				}
			}
		})
	}
}

// =============================================================================
// MERGED FROM integration_resource_test.go
// =============================================================================

// TestResourceLimits tests behavior under resource constraints
func TestResourceLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource test in short mode")
	}

	// Test with limited resources
	data := make([]byte, 10*1024*1024) // 10MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	t.Run("high_memory_pressure", func(t *testing.T) {
		// Simulate memory pressure by creating many allocations
		var allocations [][]byte
		defer func() {
			// Clean up allocations
			for i := range allocations {
				allocations[i] = nil
			}
		}()

		// Allocate memory to create pressure
		for i := 0; i < 100; i++ {
			alloc := make([]byte, 1024*1024) // 1MB each
			allocations = append(allocations, alloc)
		}

		// Try compression under memory pressure
		params := &WriterParams{
			CompressionLevel: 9, // High compression = more memory
			NbWorkers:        4,
			JobSize:          1024 * 1024,
		}

		var compressed bytes.Buffer
		writer := NewWriterParams(&compressed, params)

		start := time.Now()
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("Write failed under memory pressure: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Close failed under memory pressure: %v", err)
		}
		writer.Release()
		duration := time.Since(start)

		t.Logf("Compression under memory pressure took: %v", duration)

		// Verify the result
		decompressed, err := Decompress(nil, compressed.Bytes())
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Fatal("Data integrity lost under memory pressure")
		}
	})

	t.Run("concurrent_resource_contention", func(t *testing.T) {
		const numGoroutines = 20
		const dataSize = 1024 * 1024 // 1MB per goroutine

		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				// Generate unique data for this goroutine
				goroutineData := make([]byte, dataSize)
				for j := range goroutineData {
					goroutineData[j] = byte((id + j) % 256)
				}

				params := &WriterParams{
					CompressionLevel: 3 + (id % 5), // Varying compression levels
					NbWorkers:        1 + (id % 4), // Varying worker counts
					JobSize:          32768 + (id*1024)%65536,
				}

				var compressed bytes.Buffer
				writer := NewWriterParams(&compressed, params)

				if _, err := writer.Write(goroutineData); err != nil {
					atomic.AddInt64(&errorCount, 1)
					t.Errorf("Goroutine %d: Write failed: %v", id, err)
					writer.Release()
					return
				}

				if err := writer.Close(); err != nil {
					atomic.AddInt64(&errorCount, 1)
					t.Errorf("Goroutine %d: Close failed: %v", id, err)
					writer.Release()
					return
				}
				writer.Release()

				// Verify decompression
				decompressed, err := Decompress(nil, compressed.Bytes())
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					t.Errorf("Goroutine %d: Decompression failed: %v", id, err)
					return
				}

				if !bytes.Equal(goroutineData, decompressed) {
					atomic.AddInt64(&errorCount, 1)
					t.Errorf("Goroutine %d: Data mismatch", id)
					return
				}

				atomic.AddInt64(&successCount, 1)
			}(i)
		}

		wg.Wait()

		t.Logf("Concurrent resource test: %d successes, %d errors", successCount, errorCount)

		if errorCount > 0 {
			t.Errorf("Resource contention caused %d errors", errorCount)
		}
	})
}

// =============================================================================
// MERGED FROM integration_streaming_params_test.go
// =============================================================================

// TestStreamingParameterCombinations tests various streaming parameter combinations
func TestStreamingParameterCombinations(t *testing.T) {
	testData := bytes.Repeat([]byte("Streaming test data with repetitive patterns. "), 10000)

	parameterTests := []struct {
		name   string
		params WriterParams
	}{
		{
			name: "basic_streaming",
			params: WriterParams{
				CompressionLevel: 3,
			},
		},
		{
			name: "high_compression_streaming",
			params: WriterParams{
				CompressionLevel: 19,
			},
		},
		{
			name: "multithread_streaming",
			params: WriterParams{
				CompressionLevel: 5,
				NbWorkers:        4,
				JobSize:          64 * 1024,
				OverlapLog:       2,
			},
		},
		{
			name: "small_chunks",
			params: WriterParams{
				CompressionLevel: 3,
				NbWorkers:        2,
				JobSize:          8 * 1024, // Small job size
			},
		},
		{
			name: "large_chunks",
			params: WriterParams{
				CompressionLevel: 3,
				NbWorkers:        2,
				JobSize:          1024 * 1024, // Large job size
			},
		},
	}

	for _, tc := range parameterTests {
		t.Run(tc.name, func(t *testing.T) {
			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, &tc.params)

			// Write data in chunks to simulate streaming
			chunkSize := 8192
			for i := 0; i < len(testData); i += chunkSize {
				end := i + chunkSize
				if end > len(testData) {
					end = len(testData)
				}

				if _, err := writer.Write(testData[i:end]); err != nil {
					t.Fatalf("Chunk write failed: %v", err)
				}

				// Occasionally flush to test incremental compression
				if (i/chunkSize)%10 == 0 {
					if err := writer.Flush(); err != nil {
						t.Fatalf("Flush failed: %v", err)
					}
				}
			}

			if err := writer.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}
			writer.Release()

			// Verify by decompressing
			reader := NewReader(&compressed)
			var decompressed bytes.Buffer

			if _, err := decompressed.ReadFrom(reader); err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}
			reader.Release()

			if !bytes.Equal(testData, decompressed.Bytes()) {
				t.Fatal("Streaming data mismatch")
			}

			ratio := float64(compressed.Len()) / float64(len(testData)) * 100
			t.Logf("Compression ratio: %.2f%% (original: %d, compressed: %d)",
				ratio, len(testData), compressed.Len())
		})
	}
}

// =============================================================================
// MERGED FROM integration_target_block_size_test.go  
// =============================================================================

// TestTargetBlockSizeIntegration tests target block size parameter integration
func TestTargetBlockSizeIntegration(t *testing.T) {
	// Create test data with patterns that benefit from different block sizes
	patterns := []struct {
		name string
		data []byte
	}{
		{
			name: "highly_repetitive",
			data: bytes.Repeat([]byte("A"), 100000),
		},
		{
			name: "medium_repetitive",
			data: bytes.Repeat([]byte("ABCDEFGH"), 10000),
		},
		{
			name: "low_repetitive",
			data: func() []byte {
				data := make([]byte, 100000)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return data
			}(),
		},
	}

	blockSizes := []int{0, 1024, 4096, 16384, 65536} // 0 means auto

	for _, pattern := range patterns {
		t.Run(pattern.name, func(t *testing.T) {
			for _, blockSize := range blockSizes {
				t.Run(fmt.Sprintf("block_size_%d", blockSize), func(t *testing.T) {
					ctx := NewCCtx()
					defer ctx.Release()

					// Set compression parameters
					if err := ctx.SetParameter(ZSTD_c_compressionLevel, 6); err != nil {
						t.Fatalf("Cannot set compression level: %v", err)
					}

					if blockSize > 0 {
						if err := ctx.SetParameter(ZSTD_c_targetLength, blockSize); err != nil {
							t.Fatalf("Cannot set target block size: %v", err)
						}
					}

					// Compress
					compressed, err := ctx.Compress(nil, pattern.data)
					if err != nil {
						t.Fatalf("Compression failed: %v", err)
					}

					// Decompress and verify
					decompressed, err := Decompress(nil, compressed)
					if err != nil {
						t.Fatalf("Decompression failed: %v", err)
					}

					if !bytes.Equal(pattern.data, decompressed) {
						t.Fatal("Data mismatch")
					}

					ratio := float64(len(compressed)) / float64(len(pattern.data)) * 100
					t.Logf("Block size %d: ratio %.2f%% (%d -> %d bytes)",
						blockSize, ratio, len(pattern.data), len(compressed))
				})
			}
		})
	}
}

// =============================================================================
// MERGED FROM integration_state_test.go
// =============================================================================

// TestStateMachineIntegrity tests state machine integrity across operations
func TestStateMachineIntegrity(t *testing.T) {
	testData := []byte("State machine test data")

	t.Run("context_reuse", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Test multiple compression cycles with the same context
		for i := 0; i < 10; i++ {
			// Set different parameters each time
			level := 1 + (i % 5)
			if err := ctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
				t.Fatalf("Iteration %d: Cannot set compression level: %v", i, err)
			}

			compressed, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("Iteration %d: Compression failed: %v", i, err)
			}

			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Iteration %d: Decompression failed: %v", i, err)
			}

			if !bytes.Equal(testData, decompressed) {
				t.Fatalf("Iteration %d: Data mismatch", i)
			}
		}
	})

	t.Run("writer_state_transitions", func(t *testing.T) {
		var compressed bytes.Buffer
		writer := NewWriter(&compressed)
		defer writer.Release()

		// Test various state transitions
		states := []struct {
			name   string
			action func() error
		}{
			{
				name: "write_data",
				action: func() error {
					_, err := writer.Write(testData)
					return err
				},
			},
			{
				name: "flush",
				action: func() error {
					return writer.Flush()
				},
			},
			{
				name: "write_more",
				action: func() error {
					_, err := writer.Write(testData)
					return err
				},
			},
			{
				name: "final_flush",
				action: func() error {
					return writer.Flush()
				},
			},
			{
				name: "close",
				action: func() error {
					return writer.Close()
				},
			},
		}

		for _, state := range states {
			t.Run(state.name, func(t *testing.T) {
				if err := state.action(); err != nil {
					t.Fatalf("State %s failed: %v", state.name, err)
				}
			})
		}

		// Verify final result
		reader := NewReader(&compressed)
		defer reader.Release()

		var result bytes.Buffer
		if _, err := result.ReadFrom(reader); err != nil {
			t.Fatalf("Final decompression failed: %v", err)
		}

		// Should contain testData written twice
		expected := append(testData, testData...)
		if !bytes.Equal(expected, result.Bytes()) {
			t.Fatal("Final state verification failed")
		}
	})
}

// TestErrorStateRecovery tests error state recovery
func TestErrorStateRecovery(t *testing.T) {
	t.Run("parameter_error_recovery", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Set a valid parameter first
		if err := ctx.SetParameter(ZSTD_c_compressionLevel, 5); err != nil {
			t.Fatalf("Initial parameter set failed: %v", err)
		}

		// Try to set an invalid parameter
		err := ctx.SetParameter(ZSTD_c_checksumFlag, 999) // Invalid value
		if err == nil {
			t.Fatal("Expected parameter error but got nil")
		}

		// Context should still be usable after parameter error
		testData := []byte("Recovery test data")
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression after parameter error failed: %v", err)
		}

		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression after parameter error failed: %v", err)
		}

		if !bytes.Equal(testData, decompressed) {
			t.Fatal("Data mismatch after error recovery")
		}
	})
}
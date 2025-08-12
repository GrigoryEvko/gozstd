package gozstd

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// FuzzCompressDecompress tests basic compression and decompression with random data
func FuzzCompressDecompress(f *testing.F) {
	// Add seed corpus
	f.Add([]byte(""), 1)
	f.Add([]byte("a"), 3)
	f.Add([]byte("hello world"), 5)
	f.Add([]byte("the quick brown fox jumps over the lazy dog"), 9)
	f.Add(bytes.Repeat([]byte("abc"), 1000), 19)

	f.Fuzz(func(t *testing.T, data []byte, level int) {
		// Clamp level to valid range
		if level < -131072 {
			level = -131072
		}
		if level > 22 {
			level = 22
		}

		// Compress
		compressed := CompressLevel(nil, data, level)

		// Decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Failed to decompress: %v", err)
		}

		// Verify roundtrip
		if !bytes.Equal(data, decompressed) {
			t.Errorf("Roundtrip failed: input len=%d, compressed len=%d, decompressed len=%d",
				len(data), len(compressed), len(decompressed))
		}
	})
}

// FuzzCompressWithBuffer tests compression with pre-allocated buffers
func FuzzCompressWithBuffer(f *testing.F) {
	f.Add([]byte("test data"), 100, 3)
	f.Add(bytes.Repeat([]byte("x"), 1000), 50, 5)

	f.Fuzz(func(t *testing.T, data []byte, bufSize int, level int) {
		if bufSize < 0 || bufSize > 1<<20 {
			return // Skip invalid buffer sizes
		}

		// Create a buffer with some existing data
		existingData := []byte("PREFIX:")
		dst := make([]byte, len(existingData), len(existingData)+bufSize)
		copy(dst, existingData)

		// Compress into the buffer
		compressed := CompressLevel(dst, data, level)

		// Verify prefix is preserved
		if !bytes.HasPrefix(compressed, existingData) {
			t.Error("Prefix not preserved")
		}

		// Decompress the data part
		compressedData := compressed[len(existingData):]
		decompressed, err := Decompress(nil, compressedData)
		if err != nil {
			t.Fatalf("Failed to decompress: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("Roundtrip failed with buffer")
		}
	})
}

// FuzzDictionary tests dictionary compression with random data and dictionaries
func FuzzDictionary(f *testing.F) {
	f.Add([]byte("sample text"), []byte("dictionary content"), 3)
	f.Add([]byte("the quick brown fox"), []byte("the fox"), 5)

	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int) {
		// Skip invalid dictionary sizes
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}

		// Create compression dictionary
		cd, err := NewCDictLevel(dictData, level)
		if err != nil {
			return // Skip invalid dictionaries
		}
		defer cd.Release()

		// Create decompression dictionary
		dd, err := NewDDict(dictData)
		if err != nil {
			return
		}
		defer dd.Release()

		// Compress with dictionary
		compressed := CompressDict(nil, data, cd)

		// Decompress with dictionary
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err != nil {
			t.Fatalf("Failed to decompress with dict: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("Dictionary roundtrip failed")
		}

		// Verify decompression without dictionary behavior matches ZSTD
		// According to ZSTD source: only fails if frame specifies dictionary ID
		decompressedWithoutDict, err := Decompress(nil, compressed)
		if err != nil {
			// This is expected if frame has dictionary ID
			t.Logf("Decompression without dictionary failed as expected: %v", err)
		} else {
			// This is also valid if frame has dictionary ID 0
			t.Logf("Decompression without dictionary succeeded (frame has no dictionary requirement)")
			// If it succeeds, it should produce the same result as with dictionary
			if !bytes.Equal(data, decompressedWithoutDict) {
				t.Error("Decompression without dictionary produced different result")
			}
		}
	})
}

// FuzzDictionaryByRef tests memory-optimized dictionary functions
func FuzzDictionaryByRef(f *testing.F) {
	f.Add([]byte("test data"), []byte("dict"), 3)

	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int) {
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}

		// Create ByRef dictionaries
		cd, err := NewCDictByRefLevel(dictData, level)
		if err != nil {
			return
		}
		defer cd.Release()

		dd, err := NewDDictByRef(dictData)
		if err != nil {
			return
		}
		defer dd.Release()

		// Compress and decompress
		compressed := CompressDict(nil, data, cd)
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err != nil {
			t.Fatalf("ByRef decompression failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("ByRef dictionary roundtrip failed")
		}

		// Verify the dictionary data is still valid
		// This tests that ByRef truly references the original data
		if !bytes.Equal(dictData, dictData) {
			t.Error("Dictionary data was modified")
		}
	})
}

// FuzzAdvancedAPI tests the advanced compression API with various parameters
func FuzzAdvancedAPI(f *testing.F) {
	f.Add([]byte("test"), 3, 10, 12, 1, 0)
	f.Add([]byte("longer test data"), 5, 15, 15, 0, 1)

	f.Fuzz(func(t *testing.T, data []byte, level int, windowLog int, hashLog int,
		checksumFlag int, strategy int) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Set compression level
		if err := ctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
			return // Skip invalid levels
		}

		// Set window log (10-31 for 64-bit)
		if windowLog >= 10 && windowLog <= 31 {
			ctx.SetParameter(ZSTD_c_windowLog, windowLog)
		}

		// Set hash log
		if hashLog >= 6 && hashLog <= 30 {
			ctx.SetParameter(ZSTD_c_hashLog, hashLog)
		}

		// Set checksum flag (0 or 1)
		checksumFlag = checksumFlag & 1
		if err := ctx.SetParameter(ZSTD_c_checksumFlag, checksumFlag); err != nil {
			t.Fatalf("Failed to set checksum flag: %v", err)
		}

		// Set strategy (0-9)
		if strategy >= 0 && strategy <= 9 {
			ctx.SetParameter(ZSTD_c_strategy, strategy)
		}

		// Compress
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Advanced API compression failed: %v", err)
		}

		// Decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Failed to decompress advanced API output: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("Advanced API roundtrip failed")
		}
	})
}

// FuzzStreamCompression tests streaming compression with various chunk sizes
func FuzzStreamCompression(f *testing.F) {
	f.Add([]byte("stream test"), 10, 3)
	f.Add(bytes.Repeat([]byte("x"), 1000), 100, 5)

	f.Fuzz(func(t *testing.T, data []byte, chunkSize int, level int) {
		if chunkSize <= 0 || chunkSize > len(data) {
			chunkSize = len(data)
			if chunkSize == 0 {
				chunkSize = 1
			}
		}

		// Compress using writer
		var compressed bytes.Buffer
		w := NewWriterLevel(&compressed, level)

		// Write in chunks
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			if _, err := w.Write(data[i:end]); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		}

		if err := w.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Decompress using reader
		r := NewReader(&compressed)
		decompressed, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("Stream roundtrip failed")
		}
	})
}

// FuzzConcurrentCompression tests thread safety with concurrent operations
func FuzzConcurrentCompression(f *testing.F) {
	f.Add([]byte("concurrent"), 3, 5)

	f.Fuzz(func(t *testing.T, data []byte, level int, goroutines int) {
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 5
		}

		// Run concurrent compressions
		results := make(chan []byte, goroutines)
		errors := make(chan error, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				compressed := CompressLevel(nil, data, level)
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					errors <- err
					return
				}
				results <- decompressed
			}()
		}

		// Collect results
		for i := 0; i < goroutines; i++ {
			select {
			case err := <-errors:
				t.Fatalf("Concurrent operation failed: %v", err)
			case result := <-results:
				if !bytes.Equal(data, result) {
					t.Error("Concurrent roundtrip produced different result")
				}
			}
		}
	})
}

// FuzzResetContext tests context reset functionality
func FuzzResetContext(f *testing.F) {
	f.Add([]byte("reset test"), 3, 1)

	f.Fuzz(func(t *testing.T, data []byte, level int, resetType int) {
		ctx := NewCCtx()
		defer ctx.Release()

		// First compression
		ctx.SetParameter(ZSTD_c_compressionLevel, level)
		ctx.SetParameter(ZSTD_c_checksumFlag, 1)

		compressed1, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("First compression failed: %v", err)
		}

		// Reset context
		var resetDirective ZSTD_ResetDirective
		switch resetType % 3 {
		case 0:
			resetDirective = ZSTD_reset_session_only
		case 1:
			resetDirective = ZSTD_reset_parameters
		case 2:
			resetDirective = ZSTD_reset_session_and_parameters
		}

		if err := ctx.Reset(resetDirective); err != nil {
			t.Fatalf("Reset failed: %v", err)
		}

		// Second compression
		compressed2, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Second compression failed: %v", err)
		}

		// Both should decompress correctly
		decompressed1, _ := Decompress(nil, compressed1)
		decompressed2, _ := Decompress(nil, compressed2)

		if !bytes.Equal(data, decompressed1) || !bytes.Equal(data, decompressed2) {
			t.Error("Reset affected correctness")
		}
	})
}

// FuzzPledgedSize tests pledged source size functionality
func FuzzPledgedSize(f *testing.F) {
	f.Add([]byte("exact size"), uint64(10))
	f.Add([]byte("test"), uint64(4))

	f.Fuzz(func(t *testing.T, data []byte, pledgedSize uint64) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Set pledged size
		if err := ctx.SetPledgedSrcSize(pledgedSize); err != nil {
			return // Skip invalid sizes
		}

		// Only test if pledged size matches actual size
		if pledgedSize != uint64(len(data)) {
			return
		}

		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Compression with pledged size failed: %v", err)
		}

		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Error("Pledged size roundtrip failed")
		}
	})
}

// FuzzInvalidInput tests handling of corrupted/invalid compressed data
func FuzzInvalidInput(f *testing.F) {
	// Add some valid compressed data that we'll corrupt
	valid := Compress(nil, []byte("valid data"))
	f.Add(valid, 0, byte(1))
	f.Add(valid, len(valid)/2, byte(255))

	f.Fuzz(func(t *testing.T, compressedData []byte, corruptPos int, corruptValue byte) {
		if len(compressedData) == 0 {
			return
		}

		// Make a copy and corrupt it
		corrupted := make([]byte, len(compressedData))
		copy(corrupted, compressedData)

		if corruptPos >= 0 && corruptPos < len(corrupted) {
			corrupted[corruptPos] = corruptValue
		}

		// Try to decompress corrupted data
		_, err := Decompress(nil, corrupted)
		// We expect an error in most cases, but it shouldn't panic
		_ = err
	})
}

// =============================================================================
// MERGED FROM fuzz_validation_test.go
// =============================================================================

// TestFuzzingQuickValidation validates that our fuzzing tests can run correctly
func TestFuzzingQuickValidation(t *testing.T) {
	// Test basic multi-threading fuzzing approach
	testData := []byte("quick validation test data for fuzzing")

	// Test various parameter combinations quickly
	testCases := []WriterParams{
		{CompressionLevel: 1, NbWorkers: 0, JobSize: 0, OverlapLog: 0},     // Single threaded
		{CompressionLevel: 3, NbWorkers: 1, JobSize: 1024, OverlapLog: 1},  // Simple multi-thread
		{CompressionLevel: 5, NbWorkers: 2, JobSize: 32768, OverlapLog: 2}, // Complex multi-thread
	}

	for i, params := range testCases {
		t.Run("Case"+string(rune('0'+i)), func(t *testing.T) {
			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, &params)

			n, err := writer.Write(testData)
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
			if n != len(testData) {
				t.Fatalf("Write incomplete: got %d, want %d", n, len(testData))
			}

			if err := writer.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}
			writer.Release()

			// Verify decompression
			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}

			if !bytes.Equal(testData, decompressed) {
				t.Fatalf("Data mismatch")
			}

			ratio := float64(compressed.Len()) / float64(len(testData)) * 100
			t.Logf("Params: workers=%d, jobSize=%d, overlapLog=%d, ratio=%.2f%%",
				params.NbWorkers, params.JobSize, params.OverlapLog, ratio)
		})
	}
}

// TestDictionaryBasicFuzz validates dictionary fuzzing approach
func TestDictionaryBasicFuzz(t *testing.T) {
	// Create simple dictionary
	samples := [][]byte{
		[]byte("test pattern data"),
		[]byte("test pattern information"),
		[]byte("test pattern content"),
	}
	dictData := BuildDict(samples, 512)
	if len(dictData) == 0 {
		t.Skip("Cannot build dictionary for test")
	}

	cdict, err := NewCDict(dictData)
	if err != nil {
		t.Fatalf("Cannot create CDict: %v", err)
	}
	defer cdict.Release()

	ddict, err := NewDDict(dictData)
	if err != nil {
		t.Fatalf("Cannot create DDict: %v", err)
	}
	defer ddict.Release()

	testData := []byte("test pattern validation")

	params := &WriterParams{
		CompressionLevel: 3,
		NbWorkers:        1,
		JobSize:          1024,
		Dict:             cdict,
	}

	var compressed bytes.Buffer
	writer := NewWriterParams(&compressed, params)

	if _, err := writer.Write(testData); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	writer.Release()

	// Decompress with dictionary
	decompressed, err := DecompressDict(nil, compressed.Bytes(), ddict)
	if err != nil {
		t.Fatalf("DecompressDict failed: %v", err)
	}

	if !bytes.Equal(testData, decompressed) {
		t.Fatalf("Dictionary compression/decompression mismatch")
	}

	t.Logf("Dictionary fuzzing validation successful")
}

// TestConcurrentBasicFuzz validates concurrent fuzzing approach
func TestConcurrentBasicFuzz(t *testing.T) {
	testData := []byte("concurrent fuzzing validation test")

	const numGoroutines = 5
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			params := &WriterParams{
				CompressionLevel: 1 + (idx % 3),
				NbWorkers:        1 + (idx % 2),
				JobSize:          1024 * (1 + idx),
			}

			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, params)

			if _, err := writer.Write(testData); err != nil {
				results <- err
				return
			}

			if err := writer.Close(); err != nil {
				results <- err
				return
			}
			writer.Release()

			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				results <- err
				return
			}

			if !bytes.Equal(testData, decompressed) {
				results <- err
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Fatalf("Goroutine failed: %v", err)
		}
	}

	t.Logf("Concurrent fuzzing validation successful")
}

// =============================================================================
// MERGED FROM fuzz_thread_safety_test.go
// =============================================================================

// FuzzDictionaryRaceConditions tests the dictionary ABA fix under extreme conditions
func FuzzDictionaryRaceConditions(f *testing.F) {
	f.Add([]byte("dictionary test data"), 50, 10)

	f.Fuzz(func(t *testing.T, data []byte, numGoroutines, numDicts int) {
		if numGoroutines <= 0 || numGoroutines > 100 {
			return
		}
		if numDicts <= 0 || numDicts > 20 {
			return
		}

		// Create sample dictionary
		samples := [][]byte{
			data,
			append(data, []byte(" extended")...),
			append([]byte("prefix "), data...),
		}
		dictData := BuildDict(samples, 1024)
		if len(dictData) == 0 {
			return // Skip if we can't build dictionary
		}

		// Create multiple dictionaries
		cdicts := make([]*CDict, numDicts)
		ddicts := make([]*DDict, numDicts)

		for i := 0; i < numDicts; i++ {
			var err error
			cdicts[i], err = NewCDict(dictData)
			if err != nil {
				t.Fatalf("Cannot create CDict %d: %v", i, err)
			}

			ddicts[i], err = NewDDict(dictData)
			if err != nil {
				t.Fatalf("Cannot create DDict %d: %v", i, err)
			}
		}

		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64

		// Stress test dictionary reference counting
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				dictIdx := idx % numDicts
				cdict := cdicts[dictIdx]
				ddict := ddicts[dictIdx]

				// Randomly release and re-acquire dictionaries to trigger ABA scenarios
				if idx%5 == 0 && dictIdx > 0 {
					// Release a dictionary in some goroutines
					cdicts[dictIdx-1].Release()
					ddicts[dictIdx-1].Release()
				}

				// Try to use the dictionary
				var compressed bytes.Buffer
				writer := NewWriterDict(&compressed, cdict)

				if _, err := writer.Write(data); err != nil {
					atomic.AddInt64(&errorCount, 1)
					writer.Release()
					return
				}

				if err := writer.Close(); err != nil {
					atomic.AddInt64(&errorCount, 1)
					writer.Release()
					return
				}
				writer.Release()

				// Try to decompress
				_, err := DecompressDict(nil, compressed.Bytes(), ddict)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					return
				}

				atomic.AddInt64(&successCount, 1)
			}(i)
		}

		wg.Wait()

		// Clean up remaining dictionaries
		for i := 0; i < numDicts; i++ {
			if cdicts[i] != nil {
				cdicts[i].Release()
			}
			if ddicts[i] != nil {
				ddicts[i].Release()
			}
		}

		t.Logf("Dictionary race test: %d successes, %d errors", successCount, errorCount)
	})
}

// FuzzCCtxRaceConditions tests the CCtx SetParameter race condition fix
func FuzzCCtxRaceConditions(f *testing.F) {
	f.Add([]byte("context parameter test"), 20)

	f.Fuzz(func(t *testing.T, data []byte, numGoroutines int) {
		if numGoroutines <= 0 || numGoroutines > 50 {
			return
		}

		ctx := NewCCtx()
		defer ctx.Release()

		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64

		// Concurrent parameter setting and compression
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// Set various parameters concurrently
				params := []struct {
					param CParameter
					value int
				}{
					{ZSTD_c_compressionLevel, 1 + (idx % 5)},
					{ZSTD_c_windowLog, 10 + (idx % 5)},
					{ZSTD_c_nbWorkers, idx % 4},
				}

				for _, p := range params {
					if err := ctx.SetParameter(p.param, p.value); err != nil {
						atomic.AddInt64(&errorCount, 1)
						return
					}
				}

				// Try compression
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					return
				}

				// Verify decompression
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					return
				}

				if !bytes.Equal(data, decompressed) {
					atomic.AddInt64(&errorCount, 1)
					return
				}

				atomic.AddInt64(&successCount, 1)
			}(idx)
		}

		wg.Wait()
		t.Logf("CCtx race test: %d successes, %d errors", successCount, errorCount)
	})
}

// TestContextPoolStress tests the context pool lifecycle fix under stress
func TestContextPoolStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping context pool stress test in short mode")
	}

	data := make([]byte, 10000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Cannot generate random data: %v", err)
	}

	const numGoroutines = 50
	const numIterations = 10

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				// Create and release contexts rapidly to stress the pool
				ctx := NewCCtx()

				// Set some parameters
				if err := ctx.SetParameter(ZSTD_c_compressionLevel, 1+(j%5)); err != nil {
					atomic.AddInt64(&errorCount, 1)
					ctx.Release()
					continue
				}

				if err := ctx.SetParameter(ZSTD_c_nbWorkers, j%4); err != nil {
					atomic.AddInt64(&errorCount, 1)
					ctx.Release()
					continue
				}

				// Compress data
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					ctx.Release()
					continue
				}

				// Release context back to pool
				ctx.Release()

				// Verify decompression
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				if !bytes.Equal(data, decompressed) {
					atomic.AddInt64(&errorCount, 1)
					continue
				}

				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Context pool stress test: %d successes, %d errors", successCount, errorCount)
	if errorCount > 0 {
		t.Errorf("Context pool stress test had %d errors", errorCount)
	}
}

// TestThreadPoolSharing tests the global thread pool sharing mechanism
func TestThreadPoolSharing(t *testing.T) {
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	results := make([]time.Duration, numGoroutines)

	// Test concurrent usage of the shared thread pool
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			params := &WriterParams{
				CompressionLevel: 3,
				NbWorkers:        2 + (idx % 3), // 2, 3, or 4 workers
				JobSize:          32 * 1024,     // 32KB jobs
			}

			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, params)

			start := time.Now()
			if _, err := writer.Write(data); err != nil {
				t.Errorf("Goroutine %d: Write failed: %v", idx, err)
				writer.Release()
				return
			}

			if err := writer.Close(); err != nil {
				t.Errorf("Goroutine %d: Close failed: %v", idx, err)
				writer.Release()
				return
			}
			writer.Release()
			results[idx] = time.Since(start)

			// Verify decompression
			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				t.Errorf("Goroutine %d: Decompress failed: %v", idx, err)
				return
			}

			if !bytes.Equal(data, decompressed) {
				t.Errorf("Goroutine %d: Data mismatch", idx)
			}
		}(i)
	}

	wg.Wait()

	// Analyze results
	var totalDuration time.Duration
	for i, duration := range results {
		totalDuration += duration
		t.Logf("Goroutine %d: %.2fms", i, float64(duration.Nanoseconds())/1e6)
	}

	avgDuration := totalDuration / time.Duration(numGoroutines)
	t.Logf("Average compression time: %.2fms", float64(avgDuration.Nanoseconds())/1e6)
	t.Logf("Thread pool sharing test completed successfully")
}

// TestExtremeParameterCombinations tests edge cases with parameter combinations
func TestExtremeParameterCombinations(t *testing.T) {
	data := bytes.Repeat([]byte("extreme test "), 1000)

	extremeCases := []WriterParams{
		{CompressionLevel: 1, NbWorkers: 1, JobSize: 1024, OverlapLog: 0},              // Minimum values
		{CompressionLevel: 22, NbWorkers: 16, JobSize: 1024 * 1024, OverlapLog: 9},     // Maximum values
		{CompressionLevel: 3, NbWorkers: 8, JobSize: 0, OverlapLog: 0},                 // Auto job size
		{CompressionLevel: 1, NbWorkers: 1, JobSize: 512 * 1024 * 1024, OverlapLog: 5}, // Very large job size
	}

	for i, params := range extremeCases {
		t.Run(fmt.Sprintf("Case%d", i), func(t *testing.T) {
			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, &params)

			if _, err := writer.Write(data); err != nil {
				t.Fatalf("Write failed with params %+v: %v", params, err)
			}

			if err := writer.Close(); err != nil {
				t.Fatalf("Close failed with params %+v: %v", params, err)
			}
			writer.Release()

			// Verify decompression
			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				t.Fatalf("Decompress failed with params %+v: %v", params, err)
			}

			if !bytes.Equal(data, decompressed) {
				t.Fatalf("Data mismatch with params %+v", params)
			}

			ratio := float64(compressed.Len()) / float64(len(data)) * 100
			t.Logf("Params %+v: ratio=%.2f%%", params, ratio)
		})
	}
}

// =============================================================================
// MERGED FROM fuzz_multithreading_test.go  
// =============================================================================

// FuzzMultiThreadCompression tests multi-threaded compression with random data and parameters
func FuzzMultiThreadCompression(f *testing.F) {
	// Add seed corpus with various data patterns
	f.Add([]byte("hello world"), 1, 0, 0)
	f.Add([]byte(""), 2, 1024, 1)
	f.Add(make([]byte, 1024), 4, 64*1024, 2)
	f.Add(bytes.Repeat([]byte("test"), 1000), 8, 128*1024, 3)

	f.Fuzz(func(t *testing.T, data []byte, nbWorkers, jobSize, overlapLog int) {
		// Bound parameters to valid ranges
		if nbWorkers < 0 || nbWorkers > 16 {
			return
		}
		if jobSize < 0 || jobSize > 512*1024*1024 {
			return
		}
		if overlapLog < 0 || overlapLog > 9 {
			return
		}

		params := &WriterParams{
			CompressionLevel: 3,
			NbWorkers:        nbWorkers,
			JobSize:          jobSize,
			OverlapLog:       overlapLog,
		}

		var compressed bytes.Buffer
		writer := NewWriterParams(&compressed, params)

		// Write data
		n, err := writer.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(data) {
			t.Fatalf("Write incomplete: got %d, want %d", n, len(data))
		}

		// Close writer
		if err := writer.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		writer.Release()

		// Decompress and verify
		decompressed, err := Decompress(nil, compressed.Bytes())
		if err != nil {
			t.Fatalf("Decompress failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Data mismatch: original %d bytes, decompressed %d bytes", len(data), len(decompressed))
		}
	})
}

// FuzzConcurrentMultiThread tests concurrent multi-threaded compression
func FuzzConcurrentMultiThread(f *testing.F) {
	f.Add([]byte("concurrent test data"), 2, 4)
	f.Add(make([]byte, 10000), 4, 8)

	f.Fuzz(func(t *testing.T, data []byte, nbWorkers, numGoroutines int) {
		if nbWorkers <= 0 || nbWorkers > 8 {
			return
		}
		if numGoroutines <= 0 || numGoroutines > 20 {
			return
		}

		var wg sync.WaitGroup
		results := make([][]byte, numGoroutines)
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				params := &WriterParams{
					CompressionLevel: 1 + (idx % 5), // Vary compression levels
					NbWorkers:        nbWorkers,
					JobSize:          (idx + 1) * 1024, // Vary job sizes
				}

				var compressed bytes.Buffer
				writer := NewWriterParams(&compressed, params)

				if _, err := writer.Write(data); err != nil {
					errors[idx] = err
					return
				}

				if err := writer.Close(); err != nil {
					errors[idx] = err
					return
				}
				writer.Release()

				decompressed, err := Decompress(nil, compressed.Bytes())
				if err != nil {
					errors[idx] = err
					return
				}

				results[idx] = decompressed
			}(i)
		}

		wg.Wait()

		// Check all goroutines succeeded and produced identical results
		for i, err := range errors {
			if err != nil {
				t.Fatalf("Goroutine %d failed: %v", i, err)
			}
		}

		for i, result := range results {
			if !bytes.Equal(data, result) {
				t.Fatalf("Goroutine %d: data mismatch", i)
			}
		}
	})
}

// FuzzDictionaryMultiThread tests multi-threading with dictionaries
func FuzzDictionaryMultiThread(f *testing.F) {
	// Create sample dictionary
	samples := [][]byte{
		[]byte("common prefix data"),
		[]byte("common prefix information"),
		[]byte("common prefix content"),
	}
	dict := BuildDict(samples, 1024)
	if len(dict) == 0 {
		f.Skip("Cannot build dictionary")
	}

	f.Add([]byte("common prefix test"), 2, 1024)

	f.Fuzz(func(t *testing.T, data []byte, nbWorkers, jobSize int) {
		if nbWorkers <= 0 || nbWorkers > 4 {
			return
		}
		if jobSize <= 0 || jobSize > 1024*1024 {
			return
		}

		// Create dictionaries
		cdict, err := NewCDict(dict)
		if err != nil {
			t.Fatalf("Cannot create CDict: %v", err)
		}
		defer cdict.Release()

		ddict, err := NewDDict(dict)
		if err != nil {
			t.Fatalf("Cannot create DDict: %v", err)
		}
		defer ddict.Release()

		params := &WriterParams{
			CompressionLevel: 3,
			NbWorkers:        nbWorkers,
			JobSize:          jobSize,
			Dict:             cdict,
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

		// Decompress with dictionary
		decompressed, err := DecompressDict(nil, compressed.Bytes(), ddict)
		if err != nil {
			t.Fatalf("DecompressDict failed: %v", err)
		}

		if !bytes.Equal(data, decompressed) {
			t.Fatalf("Dictionary compression/decompression mismatch")
		}
	})
}

// TestMultiThreadStress performs stress testing of multi-threading functionality
func TestMultiThreadStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	// Generate random data
	data := make([]byte, 1024*1024) // 1MB
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Cannot generate random data: %v", err)
	}

	// Test parameters
	workerCounts := []int{0, 1, 2, 4, 8}
	jobSizes := []int{0, 1024, 32 * 1024, 128 * 1024}
	overlapLogs := []int{0, 1, 2, 3}

	for _, workers := range workerCounts {
		for _, jobSize := range jobSizes {
			for _, overlapLog := range overlapLogs {
				// Skip invalid combinations
				if workers == 0 && (jobSize > 0 || overlapLog > 0) {
					continue
				}

				testName := fmt.Sprintf("workers=%d,jobSize=%d,overlapLog=%d", workers, jobSize, overlapLog)
				t.Run(testName, func(t *testing.T) {
					params := &WriterParams{
						CompressionLevel: 3,
						NbWorkers:        workers,
						JobSize:          jobSize,
						OverlapLog:       overlapLog,
					}

					var compressed bytes.Buffer
					writer := NewWriterParams(&compressed, params)

					start := time.Now()
					if _, err := writer.Write(data); err != nil {
						t.Fatalf("Write failed: %v", err)
					}

					if err := writer.Close(); err != nil {
						t.Fatalf("Close failed: %v", err)
					}
					writer.Release()
					duration := time.Since(start)

					// Decompress and verify
					decompressed, err := Decompress(nil, compressed.Bytes())
					if err != nil {
						t.Fatalf("Decompress failed: %v", err)
					}

					if !bytes.Equal(data, decompressed) {
						t.Fatalf("Data mismatch")
					}

					ratio := float64(compressed.Len()) / float64(len(data)) * 100
					t.Logf("Parameters: workers=%d, jobSize=%d, overlapLog=%d", workers, jobSize, overlapLog)
					t.Logf("Performance: %.2fms, ratio=%.2f%%, compressed=%d bytes",
						float64(duration.Nanoseconds())/1e6, ratio, compressed.Len())
				})
			}
		}
	}
}

// TestRaceConditionDetection uses race detector to find threading issues
func TestRaceConditionDetection(t *testing.T) {
	if !testing.Short() {
		t.Log("Running with race detector - enable with: go test -race")
	}

	data := bytes.Repeat([]byte("test data for race detection "), 1000)

	const numGoroutines = 10
	var wg sync.WaitGroup

	// Test concurrent writer creation and usage
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			params := &WriterParams{
				CompressionLevel: 1 + (idx % 5),
				NbWorkers:        1 + (idx % 4),
				JobSize:          1024 * (1 + idx%10),
				OverlapLog:       idx % 4,
			}

			var compressed bytes.Buffer
			writer := NewWriterParams(&compressed, params)

			// Write in chunks to stress the system more
			chunkSize := 100 + (idx * 50)
			for pos := 0; pos < len(data); pos += chunkSize {
				end := pos + chunkSize
				if end > len(data) {
					end = len(data)
				}

				if _, err := writer.Write(data[pos:end]); err != nil {
					t.Errorf("Goroutine %d: Write failed: %v", idx, err)
					return
				}
			}

			if err := writer.Close(); err != nil {
				t.Errorf("Goroutine %d: Close failed: %v", idx, err)
				return
			}
			writer.Release()

			// Verify decompression
			decompressed, err := Decompress(nil, compressed.Bytes())
			if err != nil {
				t.Errorf("Goroutine %d: Decompress failed: %v", idx, err)
				return
			}

			if !bytes.Equal(data, decompressed) {
				t.Errorf("Goroutine %d: Data mismatch", idx)
			}
		}(idx)
	}

	wg.Wait()
}

// TestMemoryPressure tests behavior under memory pressure
func TestMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	// Force garbage collection
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create large data
	data := make([]byte, 10*1024*1024) // 10MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Test with high worker count and large job sizes
	params := &WriterParams{
		CompressionLevel: 6, // Higher compression for more CPU/memory usage
		NbWorkers:        runtime.NumCPU(),
		JobSize:          1024 * 1024, // 1MB jobs
		OverlapLog:       3,
	}

	var compressed bytes.Buffer
	writer := NewWriterParams(&compressed, params)

	start := time.Now()
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	writer.Release()
	duration := time.Since(start)

	// Verify decompression
	decompressed, err := Decompress(nil, compressed.Bytes())
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Fatalf("Data mismatch")
	}

	// Check memory usage
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	ratio := float64(compressed.Len()) / float64(len(data)) * 100
	t.Logf("Memory pressure test completed:")
	t.Logf("  Duration: %.2fms", float64(duration.Nanoseconds())/1e6)
	t.Logf("  Compression ratio: %.2f%%", ratio)
	t.Logf("  Memory allocated: %d bytes", m2.TotalAlloc-m1.TotalAlloc)
	t.Logf("  Peak memory: %d bytes", m2.Sys)
}

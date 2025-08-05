package gozstd

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
			}(i)
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
		{CompressionLevel: 1, NbWorkers: 1, JobSize: 1024, OverlapLog: 0},           // Minimum values
		{CompressionLevel: 22, NbWorkers: 16, JobSize: 1024*1024, OverlapLog: 9},    // Maximum values
		{CompressionLevel: 3, NbWorkers: 8, JobSize: 0, OverlapLog: 0},              // Auto job size
		{CompressionLevel: 1, NbWorkers: 1, JobSize: 512*1024*1024, OverlapLog: 5},  // Very large job size
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
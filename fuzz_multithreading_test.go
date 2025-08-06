package gozstd

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

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
		}(i)
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

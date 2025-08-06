package gozstd

import (
	"bytes"
	"testing"
)

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

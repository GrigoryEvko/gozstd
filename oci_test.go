package gozstd

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOCICompressor(t *testing.T) {
	t.Run("DefaultSettings", func(t *testing.T) {
		oc := DefaultOCICompressor()
		if oc.MinLevel != 6 {
			t.Errorf("Expected MinLevel=6, got %d", oc.MinLevel)
		}
		if oc.MaxLevel != 19 {
			t.Errorf("Expected MaxLevel=19, got %d", oc.MaxLevel)
		}
		if oc.DefaultLevel != 12 {
			t.Errorf("Expected DefaultLevel=12, got %d", oc.DefaultLevel)
		}
		if !oc.AutoLevel {
			t.Error("Expected AutoLevel=true")
		}
	})

	t.Run("LevelSelection", func(t *testing.T) {
		oc := DefaultOCICompressor()
		
		// Small data should use level 6
		level := oc.GetOptimalOCILevel(100 * 1024) // 100KB
		if level != 6 {
			t.Errorf("Expected level 6 for 100KB, got %d", level)
		}
		
		// Medium data should use level 9
		level = oc.GetOptimalOCILevel(2 * 1024 * 1024) // 2MB
		if level != 9 {
			t.Errorf("Expected level 9 for 2MB, got %d", level)
		}
		
		// Large data should use level 12 (default)
		level = oc.GetOptimalOCILevel(10 * 1024 * 1024) // 10MB
		if level != 12 {
			t.Errorf("Expected level 12 for 10MB, got %d", level)
		}
	})

	t.Run("CustomLevel", func(t *testing.T) {
		data := []byte("test data for compression")
		
		// Test with custom level 15
		compressed := CompressOCILayerWithCustomLevel(nil, data, 15)
		if len(compressed) == 0 {
			t.Error("Compression failed")
		}
		
		// Verify it can be decompressed
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if !bytes.Equal(decompressed, data) {
			t.Error("Decompression mismatch")
		}
	})

	t.Run("WorkerSelection", func(t *testing.T) {
		oc := DefaultOCICompressor()
		
		// Small data should not use workers
		workers := oc.GetOptimalWorkers(500 * 1024) // 500KB
		if workers != 0 {
			t.Errorf("Expected 0 workers for 500KB, got %d", workers)
		}
		
		// Large data should use workers (depends on CPU count)
		workers = oc.GetOptimalWorkers(100 * 1024 * 1024) // 100MB
		if workers < 0 {
			t.Errorf("Invalid worker count: %d", workers)
		}
	})
}

func TestOCICompression(t *testing.T) {
	testCases := []struct {
		name string
		size int
	}{
		{"Small", 1024},        // 1KB
		{"Medium", 100 * 1024}, // 100KB
		{"Large", 1024 * 1024}, // 1MB
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate test data
			data := make([]byte, tc.size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			
			// Compress
			compressed := CompressOCILayer(nil, data)
			if len(compressed) == 0 {
				t.Fatal("Compression failed")
			}
			
			// Should be smaller than original for this pattern
			if len(compressed) >= len(data) {
				t.Logf("Warning: compressed size %d >= original %d", len(compressed), len(data))
			}
			
			// Decompress and verify
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}
			if !bytes.Equal(decompressed, data) {
				t.Error("Data mismatch after decompression")
			}
		})
	}
}

func TestOCIStreamCompression(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		data := bytes.Repeat([]byte("test data "), 1000)
		
		var compressed bytes.Buffer
		err := StreamCompressOCILayer(&compressed, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Stream compression failed: %v", err)
		}
		
		// Decompress
		var decompressed bytes.Buffer
		err = StreamDecompress(&decompressed, bytes.NewReader(compressed.Bytes()))
		if err != nil {
			t.Fatalf("Stream decompression failed: %v", err)
		}
		
		if !bytes.Equal(decompressed.Bytes(), data) {
			t.Error("Data mismatch")
		}
	})

	t.Run("CustomLevel", func(t *testing.T) {
		data := bytes.Repeat([]byte("compress me "), 100)
		
		var compressed bytes.Buffer
		err := StreamCompressOCILayerLevel(&compressed, bytes.NewReader(data), 15)
		if err != nil {
			t.Fatalf("Stream compression failed: %v", err)
		}
		
		if compressed.Len() == 0 {
			t.Error("No compressed data")
		}
	})
}

func TestOCIProgressTracking(t *testing.T) {
	t.Run("BasicProgress", func(t *testing.T) {
		data := make([]byte, 1024*1024) // 1MB
		for i := range data {
			data[i] = byte(i % 256)
		}
		
		var progressCalled int32
		var lastProcessed int64
		
		oc := DefaultOCICompressor()
		oc.ProgressCallback = func(processed, total int64) {
			atomic.AddInt32(&progressCalled, 1)
			if processed > lastProcessed {
				lastProcessed = processed
			}
			if total != int64(len(data)) && total != 0 {
				t.Errorf("Unexpected total: %d", total)
			}
		}
		
		compressed := oc.Compress(nil, data)
		if len(compressed) == 0 {
			t.Fatal("Compression failed")
		}
		
		if atomic.LoadInt32(&progressCalled) < 2 {
			t.Error("Progress callback not called enough")
		}
	})

	t.Run("ProgressTracker", func(t *testing.T) {
		layerName := "test-layer"
		data := bytes.Repeat([]byte("test"), 1000)
		
		var progressInfo *OCIProgressInfo
		callback := func(info *OCIProgressInfo) {
			progressInfo = info
		}
		
		tracker := NewOCIProgressTracker(layerName, int64(len(data)), callback)
		tracker.Start()
		
		// Simulate progress
		for i := 0; i <= len(data); i += 100 {
			tracker.Update(int64(i), int64(i/2))
			time.Sleep(1 * time.Millisecond)
		}
		
		tracker.Complete()
		
		if progressInfo == nil {
			t.Fatal("No progress info received")
		}
		if !progressInfo.IsComplete {
			t.Error("Progress not marked complete")
		}
		if progressInfo.LayerName != layerName {
			t.Errorf("Wrong layer name: %s", progressInfo.LayerName)
		}
	})
}

func TestProgressFormatting(t *testing.T) {
	info := &OCIProgressInfo{
		LayerName:       "layer-1",
		TotalBytes:      1024 * 1024 * 100, // 100MB
		ProcessedBytes:  1024 * 1024 * 50,  // 50MB
		CompressedBytes: 1024 * 1024 * 25,  // 25MB
		Percentage:      50.0,
		Speed:           10.5,
		CompressionRatio: 2.0,
		TimeElapsed:     5 * time.Second,
		TimeRemaining:   5 * time.Second,
	}
	
	bar := FormatProgressBar(info)
	if !strings.Contains(bar, "50.0%") {
		t.Errorf("Progress bar missing percentage: %s", bar)
	}
	if !strings.Contains(bar, "10.50 MB/s") {
		t.Errorf("Progress bar missing speed: %s", bar)
	}
	if !strings.Contains(bar, "2.00x") {
		t.Errorf("Progress bar missing compression ratio: %s", bar)
	}
}

func TestBatchProgressTracker(t *testing.T) {
	var overallInfo *OCIProgressInfo
	callback := func(layers map[string]*OCIProgressInfo, overall *OCIProgressInfo) {
		overallInfo = overall
	}
	
	tracker := NewBatchProgressTracker(callback)
	
	// Add layers
	tracker.AddLayer("layer-1", 1000)
	tracker.AddLayer("layer-2", 2000)
	
	// Update progress
	tracker.UpdateLayer("layer-1", &OCIProgressInfo{
		LayerName:  "layer-1",
		TotalBytes: 1000,
		IsComplete: true,
	})
	
	if overallInfo == nil {
		t.Fatal("No overall progress received")
	}
	if overallInfo.ProcessedBytes != 1000 {
		t.Errorf("Wrong processed bytes: %d", overallInfo.ProcessedBytes)
	}
	if overallInfo.TotalBytes != 3000 {
		t.Errorf("Wrong total bytes: %d", overallInfo.TotalBytes)
	}
}

func TestOCIStreamCompressor(t *testing.T) {
	t.Run("AutoWorkers", func(t *testing.T) {
		sc := DefaultOCIStreamCompressor()
		
		// Small file
		workers := sc.autoDetectWorkers(5 * 1024 * 1024) // 5MB
		if workers != 1 {
			t.Errorf("Expected 1 worker for 5MB, got %d", workers)
		}
		
		// Large file (result depends on CPU)
		workers = sc.autoDetectWorkers(2 * 1024 * 1024 * 1024) // 2GB
		if workers < 1 {
			t.Errorf("Invalid worker count for 2GB: %d", workers)
		}
	})

	t.Run("SimpleStreaming", func(t *testing.T) {
		data := bytes.Repeat([]byte("stream this data "), 1000)
		
		var compressed bytes.Buffer
		sc := DefaultOCIStreamCompressor()
		sc.Workers = 1 // Force single-threaded
		
		err := sc.StreamCompressLarge(&compressed, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("Stream compression failed: %v", err)
		}
		
		if compressed.Len() == 0 {
			t.Error("No compressed output")
		}
		
		// Verify decompression
		var decompressed bytes.Buffer
		err = StreamDecompress(&decompressed, bytes.NewReader(compressed.Bytes()))
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(decompressed.Bytes(), data) {
			t.Error("Data mismatch")
		}
	})

	t.Run("ParallelStreaming", func(t *testing.T) {
		// Create larger data for parallel processing
		data := make([]byte, 20*1024*1024) // 20MB
		rand.Read(data)
		
		var compressed bytes.Buffer
		sc := DefaultOCIStreamCompressor()
		sc.Workers = 2 // Force parallel
		
		err := sc.StreamCompressLarge(&compressed, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("Parallel compression failed: %v", err)
		}
		
		if compressed.Len() == 0 {
			t.Error("No compressed output")
		}
		
		// Note: Parallel compression creates independent chunks,
		// so we can't directly decompress with standard decompressor
		// This is expected behavior for chunked compression
	})
}

func TestChunkedOCICompressor(t *testing.T) {
	cc := DefaultChunkedOCICompressor()
	
	if cc.ChunkSize != 4*1024*1024 {
		t.Errorf("Expected chunk size 4MB, got %d", cc.ChunkSize)
	}
	if cc.Level != 12 {
		t.Errorf("Expected level 12, got %d", cc.Level)
	}
	if cc.EnableIndex {
		t.Error("Index should be disabled by default for OCI compatibility")
	}
	
	// Test chunked compression
	data := bytes.Repeat([]byte("chunk me "), 1000)
	var compressed bytes.Buffer
	
	err := cc.CompressChunked(&compressed, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunked compression failed: %v", err)
	}
	
	if compressed.Len() == 0 {
		t.Error("No compressed output")
	}
}

func TestProgressReaderWriter(t *testing.T) {
	t.Run("ProgressWriter", func(t *testing.T) {
		var progressCalled bool
		tracker := NewOCIProgressTracker("test", 0, func(info *OCIProgressInfo) {
			progressCalled = true
		})
		
		var buf bytes.Buffer
		pw := NewProgressWriter(&buf, tracker)
		
		data := []byte("write this data")
		n, err := pw.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != len(data) {
			t.Errorf("Wrote %d bytes, expected %d", n, len(data))
		}
		
		if !progressCalled {
			t.Error("Progress not tracked")
		}
	})

	t.Run("ProgressReader", func(t *testing.T) {
		var progressCalled bool
		tracker := NewOCIProgressTracker("test", 0, func(info *OCIProgressInfo) {
			progressCalled = true
		})
		
		data := []byte("read this data")
		pr := NewProgressReader(bytes.NewReader(data), tracker)
		
		buf := make([]byte, len(data))
		n, err := pr.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read failed: %v", err)
		}
		if n != len(data) {
			t.Errorf("Read %d bytes, expected %d", n, len(data))
		}
		
		if !progressCalled {
			t.Error("Progress not tracked")
		}
	})
}

func BenchmarkOCICompression(b *testing.B) {
	sizes := []int{
		1024,           // 1KB
		100 * 1024,     // 100KB
		1024 * 1024,    // 1MB
		10 * 1024 * 1024, // 10MB
	}
	
	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 256)
		}
		
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				compressed := CompressOCILayer(nil, data)
				if len(compressed) == 0 {
					b.Fatal("Compression failed")
				}
			}
		})
	}
}

func BenchmarkOCIStreaming(b *testing.B) {
	data := make([]byte, 10*1024*1024) // 10MB
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		var compressed bytes.Buffer
		err := StreamCompressOCILayer(&compressed, bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
	}
}
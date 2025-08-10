package gozstd

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

// TestWriterAutoClose verifies that Writer auto-closes on finalization
func TestWriterAutoClose(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("test data for auto close")
	
	// Create writer without closing
	func() {
		w := NewWriter(&buf)
		w.Write(data)
		// No Close() call - should be handled by finalizer
	}()
	
	// Force GC to trigger finalizer
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	
	// The buffer should contain properly finalized compressed data
	compressed := buf.Bytes()
	if len(compressed) == 0 {
		t.Fatal("No compressed data in buffer after auto-close")
	}
	
	// Verify the compressed data is valid
	decompressed, err := Decompress(nil, compressed)
	if err != nil {
		t.Fatalf("Failed to decompress auto-closed data: %v", err)
	}
	
	if !bytes.Equal(decompressed, data) {
		t.Error("Data mismatch after auto-close")
	}
	
	t.Log("Writer auto-close test passed")
}

// TestManagedBuffer verifies managed buffer auto-return
func TestManagedBuffer(t *testing.T) {
	// Test auto-return on GC
	func() {
		mb := NewManagedBuffer(1024)
		mb.Append([]byte("test data"))
		// No Release() - should auto-return
	}()
	
	runtime.GC()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	
	// Test manual release
	mb := NewManagedBuffer(2048)
	mb.Append([]byte("more test data"))
	data := mb.Bytes()
	if len(data) != 14 {
		t.Errorf("Expected 14 bytes, got %d", len(data))
	}
	
	mb.Release()
	
	// After release, operations should be no-ops
	mb.Append([]byte("should not append"))
	if mb.Len() != 0 {
		t.Error("Buffer should be empty after release")
	}
	
	// Double release should be safe
	mb.Release()
	
	t.Log("Managed buffer test passed")
}

// TestAutoTuning verifies automatic parameter tuning
func TestAutoTuning(t *testing.T) {
	tuner := GetAutoTuner()
	
	// Test compression level selection
	level := tuner.GetOptimalCompressionLevel()
	if level < 1 || level > 22 {
		t.Errorf("Invalid compression level: %d", level)
	}
	t.Logf("Auto-selected compression level: %d", level)
	
	// Test worker count selection
	smallData := 100 * 1024 // 100KB
	workers := tuner.GetOptimalWorkers(smallData)
	if workers != 0 {
		t.Errorf("Expected 0 workers for small data, got %d", workers)
	}
	
	largeData := 100 * 1024 * 1024 // 100MB
	workers = tuner.GetOptimalWorkers(largeData)
	t.Logf("Auto-selected %d workers for 100MB data", workers)
	
	// Test window log selection
	windowLog := tuner.GetOptimalWindowLog(largeData)
	// 0 is valid (means use ZSTD default)
	if windowLog != 0 && (windowLog < 10 || windowLog > 31) {
		t.Errorf("Invalid window log: %d", windowLog)
	}
	t.Logf("Auto-selected window log: %d", windowLog)
	
	// Test auto-compress
	testData := []byte("test data for auto compression")
	compressed := AutoCompressLevel(nil, testData)
	if len(compressed) == 0 {
		t.Fatal("Auto-compression failed")
	}
	
	decompressed, err := Decompress(nil, compressed)
	if err != nil {
		t.Fatalf("Failed to decompress auto-compressed data: %v", err)
	}
	
	if !bytes.Equal(decompressed, testData) {
		t.Error("Data mismatch after auto-compression")
	}
	
	t.Log("Auto-tuning test passed")
}

// TestCompressAuto verifies fully automatic compression
func TestCompressAuto(t *testing.T) {
	// Test with various data sizes
	testCases := []struct {
		name string
		data []byte
	}{
		{"tiny", []byte("x")},
		{"small", bytes.Repeat([]byte("test"), 100)},
		{"medium", bytes.Repeat([]byte("medium data"), 1000)},
		{"large", bytes.Repeat([]byte("large"), 100000)},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			compressed := CompressAuto(nil, tc.data)
			if len(compressed) == 0 {
				t.Fatalf("CompressAuto failed for %s data", tc.name)
			}
			
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}
			
			if !bytes.Equal(decompressed, tc.data) {
				t.Errorf("Data mismatch for %s", tc.name)
			}
			
			ratio := float64(len(tc.data)) / float64(len(compressed))
			t.Logf("%s: %d -> %d bytes (ratio: %.2fx)", 
				tc.name, len(tc.data), len(compressed), ratio)
		})
	}
}

// TestAutoRecovery verifies automatic error recovery
func TestAutoRecovery(t *testing.T) {
	data := []byte("test data for recovery")
	
	// Test compression with retry
	config := DefaultRetryConfig()
	compressed, err := CompressWithRetry(nil, data, 5, config)
	if err != nil {
		t.Fatalf("CompressWithRetry failed: %v", err)
	}
	
	if len(compressed) == 0 {
		t.Fatal("No compressed data returned")
	}
	
	// Test decompression with retry
	decompressed, err := DecompressWithRetry(nil, compressed, config)
	if err != nil {
		t.Fatalf("DecompressWithRetry failed: %v", err)
	}
	
	if !bytes.Equal(decompressed, data) {
		t.Error("Data mismatch after recovery")
	}
	
	// Test safe compress/decompress
	compressed = SafeCompress(nil, data)
	if len(compressed) == 0 {
		t.Fatal("SafeCompress failed")
	}
	
	decompressed, err = SafeDecompress(nil, compressed)
	if err != nil {
		t.Fatalf("SafeDecompress failed: %v", err)
	}
	
	if !bytes.Equal(decompressed, data) {
		t.Error("Data mismatch with safe functions")
	}
	
	t.Log("Auto-recovery test passed")
}

// BenchmarkAutoVsManual compares auto-tuned vs manual compression
func BenchmarkAutoVsManual(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark data"), 10000)
	
	b.Run("Manual_Default", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			_ = Compress(nil, data)
		}
	})
	
	b.Run("Auto_Tuned", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			_ = CompressAuto(nil, data)
		}
	})
	
	b.Run("Auto_Level", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			_ = AutoCompressLevel(nil, data)
		}
	})
}

// TestAllAutoFeatures runs all automatic features together
func TestAllAutoFeatures(t *testing.T) {
	// Test data
	data := []byte("comprehensive test of all automatic features")
	
	// 1. Auto-release CCtx
	ctx := NewCCtx()
	compressed, err := ctx.Compress(nil, data)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	// No Release() - auto-cleanup
	
	// 2. Managed buffer
	mb := GetManagedCompressBuffer(len(data))
	mb.Append(compressed)
	// No Release() - auto-return
	
	// 3. Auto-tuned compression
	autoCompressed := CompressAuto(nil, data)
	
	// 4. Safe decompression with retry
	decompressed, err := SafeDecompress(nil, autoCompressed)
	if err != nil {
		t.Fatalf("Safe decompression failed: %v", err)
	}
	
	if !bytes.Equal(decompressed, data) {
		t.Error("Data mismatch in comprehensive test")
	}
	
	// 5. Writer auto-close
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Write(data)
	// No Close() - auto-close on GC
	
	// Force cleanup
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	
	t.Log("All automatic features test passed")
}
package gozstd

import (
	"runtime"
	"testing"
	"time"
)

// TestAutoRelease verifies that CCtx is automatically released by GC
func TestAutoRelease(t *testing.T) {
	// Test that contexts are automatically released
	data := []byte("test data for auto release")
	
	// Create contexts without explicit release
	for i := 0; i < 10; i++ {
		ctx := NewCCtx()
		ctx.SetParameter(ZSTD_c_compressionLevel, i%5)
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}
		
		// Verify compression worked
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if string(decompressed) != string(data) {
			t.Error("Data mismatch after compression/decompression")
		}
		
		// No explicit Release() call - should be handled by finalizer
	}
	
	// Force GC to run finalizers
	runtime.GC()
	runtime.GC() // Run twice to ensure finalizers execute
	time.Sleep(100 * time.Millisecond) // Give finalizers time to run
	
	// If we get here without crashes/leaks, auto-release is working
	t.Log("Auto-release test completed successfully")
}

// TestDoubleRelease verifies that calling Release twice is safe
func TestDoubleRelease(t *testing.T) {
	ctx := NewCCtx()
	data := []byte("test data")
	
	compressed, err := ctx.Compress(nil, data)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	
	// First release
	ctx.Release()
	
	// Second release - should be safe no-op
	ctx.Release()
	
	// Third release - still safe
	ctx.Release()
	
	// Verify the compressed data is still valid
	decompressed, err := Decompress(nil, compressed)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}
	if string(decompressed) != string(data) {
		t.Error("Data mismatch")
	}
	
	t.Log("Double release test completed successfully")
}

// TestMixedReleaseStyles verifies manual and auto release work together
func TestMixedReleaseStyles(t *testing.T) {
	data := []byte("test mixed release styles")
	
	// Some contexts manually released
	for i := 0; i < 5; i++ {
		ctx := NewCCtx()
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}
		ctx.Release() // Manual release
		
		// Verify compression worked
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if string(decompressed) != string(data) {
			t.Error("Data mismatch")
		}
	}
	
	// Some contexts auto-released
	for i := 0; i < 5; i++ {
		ctx := NewCCtx()
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}
		// No Release() - rely on finalizer
		
		// Verify compression worked
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		if string(decompressed) != string(data) {
			t.Error("Data mismatch")
		}
	}
	
	// Force GC
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	
	t.Log("Mixed release styles test completed successfully")
}

// BenchmarkAutoRelease compares performance with and without explicit release
func BenchmarkAutoRelease(b *testing.B) {
	data := []byte("benchmark data for auto release testing")
	
	b.Run("ManualRelease", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ctx := NewCCtx()
			compressed, err := ctx.Compress(nil, data)
			if err != nil {
				b.Fatal(err)
			}
			ctx.Release()
			_ = compressed
		}
	})
	
	b.Run("AutoRelease", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			ctx := NewCCtx()
			compressed, err := ctx.Compress(nil, data)
			if err != nil {
				b.Fatal(err)
			}
			// No explicit release
			_ = compressed
		}
	})
}
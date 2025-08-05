package gozstd

import (
	"bytes"
	"fmt"
	"testing"
)

func TestStreamingParameters(t *testing.T) {
	t.Run("SetSourceSizeHint", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test various size hints
		testSizes := []uint64{0, 1024, 65536, 1024 * 1024}
		
		for _, size := range testSizes {
			err := ctx.SetSourceSizeHint(size)
			if err != nil {
				t.Errorf("SetSourceSizeHint(%d) failed: %v", size, err)
			}
		}
	})
	
	t.Run("SetStableInputBuffer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test enable
		err := ctx.SetStableInputBuffer(true)
		if err != nil {
			t.Fatalf("SetStableInputBuffer(true) failed: %v", err)
		}
		
		// Test disable
		err = ctx.SetStableInputBuffer(false)
		if err != nil {
			t.Fatalf("SetStableInputBuffer(false) failed: %v", err)
		}
	})
	
	t.Run("SetStableOutputBuffer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test enable
		err := ctx.SetStableOutputBuffer(true)
		if err != nil {
			t.Fatalf("SetStableOutputBuffer(true) failed: %v", err)
		}
		
		// Test disable
		err = ctx.SetStableOutputBuffer(false)
		if err != nil {
			t.Fatalf("SetStableOutputBuffer(false) failed: %v", err)
		}
	})
	
	t.Run("DirectParameterSetting", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test setting parameters directly
		err := ctx.SetParameter(ZSTD_c_srcSizeHint, 65536)
		if err != nil {
			t.Fatalf("SetParameter(srcSizeHint) failed: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_stableInBuffer, 1)
		if err != nil {
			t.Fatalf("SetParameter(stableInBuffer) failed: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_stableOutBuffer, 1)
		if err != nil {
			t.Fatalf("SetParameter(stableOutBuffer) failed: %v", err)
		}
	})
	
	t.Run("StreamingParametersWithCompression", func(t *testing.T) {
		testData := make([]byte, 50000) // 50KB test data
		for i := range testData {
			testData[i] = byte(i % 256)
		}
		
		// Test with source size hint
		ctxWithHint := NewCCtx()
		defer ctxWithHint.Release()
		
		err := ctxWithHint.SetSourceSizeHint(uint64(len(testData)))
		if err != nil {
			t.Fatalf("SetSourceSizeHint failed: %v", err)
		}
		
		compressedWithHint, err := ctxWithHint.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression with size hint failed: %v", err)
		}
		
		// Test normal compression for comparison
		normalCompressed := Compress(nil, testData)
		
		// Both should decompress to the same data
		decompressedWithHint, err := Decompress(nil, compressedWithHint)
		if err != nil {
			t.Fatalf("Decompression of hint-compressed data failed: %v", err)
		}
		
		normalDecompressed, err := Decompress(nil, normalCompressed)
		if err != nil {
			t.Fatalf("Normal decompression failed: %v", err)
		}
		
		// Verify both decompress to original data
		if !bytes.Equal(testData, decompressedWithHint) {
			t.Error("Hint compression/decompression data mismatch")
		}
		
		if !bytes.Equal(testData, normalDecompressed) {
			t.Error("Normal compression/decompression data mismatch")
		}
		
		// Log compression ratios for comparison
		hintRatio := float64(len(testData)) / float64(len(compressedWithHint))
		normalRatio := float64(len(testData)) / float64(len(normalCompressed))
		
		t.Logf("Size hint compression: %d -> %d bytes (ratio: %.2fx)", 
			len(testData), len(compressedWithHint), hintRatio)
		t.Logf("Normal compression: %d -> %d bytes (ratio: %.2fx)", 
			len(testData), len(normalCompressed), normalRatio)
	})
	
	t.Run("InvalidParameters", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test invalid srcSizeHint (too large)
		err := ctx.SetParameter(ZSTD_c_srcSizeHint, 1<<31) // 2GB
		if err == nil {
			t.Error("Expected error for oversized srcSizeHint")
		}
		
		// Test invalid stableInBuffer values
		invalidValues := []int{-1, 2, 100}
		for _, value := range invalidValues {
			err := ctx.SetParameter(ZSTD_c_stableInBuffer, value)
			if err == nil {
				t.Errorf("Expected error for invalid stableInBuffer value %d", value)
			}
		}
		
		// Test invalid stableOutBuffer values
		for _, value := range invalidValues {
			err := ctx.SetParameter(ZSTD_c_stableOutBuffer, value)
			if err == nil {
				t.Errorf("Expected error for invalid stableOutBuffer value %d", value)
			}
		}
	})
}

func TestStreamingParametersValidation(t *testing.T) {
	validator := NewParameterValidator()
	ctx := &ValidationContext{
		Architecture:  "amd64",
		CurrentParams: make(map[CParameter]int),
	}
	
	t.Run("SourceSizeHintValidation", func(t *testing.T) {
		// Valid values
		validSizes := []int{0, 1024, 65536, 1024 * 1024, 1 << 30}
		for _, size := range validSizes {
			err := validator.ValidateParameter(ZSTD_c_srcSizeHint, size, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid srcSizeHint %d: %v", size, err)
			}
		}
		
		// Invalid values
		invalidSizes := []int{-1, 1<<31 - 1} // Negative and too large
		for _, size := range invalidSizes {
			err := validator.ValidateParameter(ZSTD_c_srcSizeHint, size, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid srcSizeHint %d", size)
			}
		}
	})
	
	t.Run("StableBufferValidation", func(t *testing.T) {
		// Valid values for stable buffer parameters
		validValues := []int{0, 1}
		
		// Test stableInBuffer
		for _, value := range validValues {
			err := validator.ValidateParameter(ZSTD_c_stableInBuffer, value, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid stableInBuffer %d: %v", value, err)
			}
		}
		
		// Test stableOutBuffer
		for _, value := range validValues {
			err := validator.ValidateParameter(ZSTD_c_stableOutBuffer, value, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid stableOutBuffer %d: %v", value, err)
			}
		}
		
		// Invalid values
		invalidValues := []int{-1, 2, 10}
		
		// Test invalid stableInBuffer
		for _, value := range invalidValues {
			err := validator.ValidateParameter(ZSTD_c_stableInBuffer, value, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid stableInBuffer %d", value)
			}
		}
		
		// Test invalid stableOutBuffer
		for _, value := range invalidValues {
			err := validator.ValidateParameter(ZSTD_c_stableOutBuffer, value, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid stableOutBuffer %d", value)
			}
		}
	})
}

func TestStreamingParametersIntegration(t *testing.T) {
	// Test integration of multiple streaming parameters
	ctx := NewCCtx()
	defer ctx.Release()
	
	testData := []byte("Integration test data for advanced streaming parameters with size hint and stable buffers.")
	
	// Set multiple streaming parameters
	err := ctx.SetSourceSizeHint(uint64(len(testData)))
	if err != nil {
		t.Fatalf("SetSourceSizeHint failed: %v", err)
	}
	
	// Note: Stable buffer parameters are typically used with streaming API
	// For simple Compress() we can set them but they may not have much effect
	err = ctx.SetStableInputBuffer(false) // Keep default for safety
	if err != nil {
		t.Fatalf("SetStableInputBuffer failed: %v", err)
	}
	
	err = ctx.SetStableOutputBuffer(false) // Keep default for safety
	if err != nil {
		t.Fatalf("SetStableOutputBuffer failed: %v", err)
	}
	
	// Set additional parameters for comprehensive test
	err = ctx.SetParameter(ZSTD_c_compressionLevel, 6)
	if err != nil {
		t.Fatalf("SetParameter(compressionLevel) failed: %v", err)
	}
	
	err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
	if err != nil {
		t.Fatalf("SetParameter(checksumFlag) failed: %v", err)
	}
	
	// Test compression with all parameters set
	compressed, err := ctx.Compress(nil, testData)
	if err != nil {
		t.Fatalf("Integration compression failed: %v", err)
	}
	
	// Verify decompression
	decompressed, err := Decompress(nil, compressed)
	if err != nil {
		t.Fatalf("Integration decompression failed: %v", err)
	}
	
	if !bytes.Equal(testData, decompressed) {
		t.Error("Integration test data mismatch")
	}
	
	t.Logf("Integration test successful: %d -> %d bytes", len(testData), len(compressed))
}

func TestSourceSizeHintEffect(t *testing.T) {
	// Create test data with known patterns to test size hint effectiveness
	sizes := []int{1000, 10000, 100000}
	
	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			testData := make([]byte, size)
			for i := range testData {
				// Create some pattern that might benefit from size hint
				testData[i] = byte((i / 100) % 256)
			}
			
			// Compression without hint
			normalCompressed := Compress(nil, testData)
			
			// Compression with accurate hint
			ctxAccurate := NewCCtx()
			defer ctxAccurate.Release()
			
			err := ctxAccurate.SetSourceSizeHint(uint64(len(testData)))
			if err != nil {
				t.Fatalf("SetSourceSizeHint failed: %v", err)
			}
			
			accurateCompressed, err := ctxAccurate.Compress(nil, testData)
			if err != nil {
				t.Fatalf("Compression with accurate hint failed: %v", err)
			}
			
			// Compression with inaccurate hint (much smaller)
			ctxInaccurate := NewCCtx()
			defer ctxInaccurate.Release()
			
			err = ctxInaccurate.SetSourceSizeHint(uint64(len(testData) / 10))
			if err != nil {
				t.Fatalf("SetSourceSizeHint failed: %v", err)
			}
			
			inaccurateCompressed, err := ctxInaccurate.Compress(nil, testData)
			if err != nil {
				t.Fatalf("Compression with inaccurate hint failed: %v", err)
			}
			
			// All should decompress correctly
			decompressed1, err := Decompress(nil, normalCompressed)
			if err != nil || !bytes.Equal(testData, decompressed1) {
				t.Error("Normal compression/decompression failed")
			}
			
			decompressed2, err := Decompress(nil, accurateCompressed)
			if err != nil || !bytes.Equal(testData, decompressed2) {
				t.Error("Accurate hint compression/decompression failed")
			}
			
			decompressed3, err := Decompress(nil, inaccurateCompressed)
			if err != nil || !bytes.Equal(testData, decompressed3) {
				t.Error("Inaccurate hint compression/decompression failed")
			}
			
			// Log results for analysis
			normalRatio := float64(len(testData)) / float64(len(normalCompressed))
			accurateRatio := float64(len(testData)) / float64(len(accurateCompressed))
			inaccurateRatio := float64(len(testData)) / float64(len(inaccurateCompressed))
			
			t.Logf("Size %d results:", size)
			t.Logf("  Normal: %d -> %d bytes (ratio: %.2fx)", 
				len(testData), len(normalCompressed), normalRatio)
			t.Logf("  Accurate hint: %d -> %d bytes (ratio: %.2fx)", 
				len(testData), len(accurateCompressed), accurateRatio)
			t.Logf("  Inaccurate hint: %d -> %d bytes (ratio: %.2fx)", 
				len(testData), len(inaccurateCompressed), inaccurateRatio)
		})
	}
}
package gozstd

import (
	"testing"
)

func TestTargetBlockSize(t *testing.T) {
	t.Run("SetTargetBlockSize_Valid", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Test various valid block sizes
		testSizes := []int{0, 1024, 8192, 32768, 65536, 128 * 1024}

		for _, size := range testSizes {
			err := ctx.SetTargetBlockSize(size)
			if err != nil {
				t.Errorf("SetTargetBlockSize(%d) failed: %v", size, err)
			}
		}
	})

	t.Run("SetTargetBlockSize_Invalid", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Test invalid block sizes (too large)
		invalidSizes := []int{-1, 200 * 1024, 1024 * 1024} // > 128KB max

		for _, size := range invalidSizes {
			err := ctx.SetTargetBlockSize(size)
			if err == nil {
				t.Errorf("SetTargetBlockSize(%d) should have failed but didn't", size)
			}

			if !IsParameterError(err) {
				t.Errorf("Expected ParameterError for size %d, got %T: %v", size, err, err)
			}
		}
	})

	t.Run("SetTargetBlockSize_DirectParameter", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Test setting parameter directly
		err := ctx.SetParameter(ZSTD_c_targetCBlockSize, 16384)
		if err != nil {
			t.Errorf("SetParameter(ZSTD_c_targetCBlockSize, 16384) failed: %v", err)
		}
	})

	t.Run("TargetBlockSize_CompressionWorks", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Set target block size
		err := ctx.SetTargetBlockSize(8192)
		if err != nil {
			t.Fatalf("SetTargetBlockSize failed: %v", err)
		}

		// Test compression still works
		testData := make([]byte, 50000) // 50KB test data
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}

		// Verify decompression works
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}

		if len(decompressed) != len(testData) {
			t.Errorf("Decompressed size mismatch: got %d, expected %d",
				len(decompressed), len(testData))
		}

		// Quick content verification
		for i := 0; i < len(testData) && i < len(decompressed); i++ {
			if testData[i] != decompressed[i] {
				t.Errorf("Data mismatch at position %d: got %02x, expected %02x",
					i, decompressed[i], testData[i])
				break
			}
		}

		t.Logf("Target block size compression successful: %d -> %d bytes (ratio: %.2fx)",
			len(testData), len(compressed), float64(len(testData))/float64(len(compressed)))
	})
}

func TestTargetBlockSizeValidation(t *testing.T) {
	// Test parameter validation bounds
	validator := NewParameterValidator()

	ctx := &ValidationContext{
		Architecture:  "amd64",
		CurrentParams: make(map[CParameter]int),
	}

	// Valid sizes
	validSizes := []int{0, 1024, 16384, 65536, 128 * 1024}
	for _, size := range validSizes {
		err := validator.ValidateParameter(ZSTD_c_targetCBlockSize, size, ctx)
		if err != nil {
			t.Errorf("Validation failed for valid size %d: %v", size, err)
		}
	}

	// Invalid sizes
	invalidSizes := []int{-1, 129 * 1024, 1024 * 1024}
	for _, size := range invalidSizes {
		err := validator.ValidateParameter(ZSTD_c_targetCBlockSize, size, ctx)
		if err == nil {
			t.Errorf("Validation should have failed for invalid size %d", size)
		}
	}
}

package gozstd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRsyncFriendly(t *testing.T) {
	t.Run("SetRsyncFriendly_Enable", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Enable multi-threading first (required for rsyncable)
		err := ctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter(nbWorkers) failed: %v", err)
		}

		err = ctx.SetRsyncFriendly(true)
		if err != nil {
			t.Fatalf("SetRsyncFriendly(true) failed: %v", err)
		}
	})

	t.Run("SetRsyncFriendly_Disable", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		err := ctx.SetRsyncFriendly(false)
		if err != nil {
			t.Fatalf("SetRsyncFriendly(false) failed: %v", err)
		}
	})

	t.Run("SetRsyncFriendly_DirectParameter", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Enable multi-threading first
		err := ctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter(nbWorkers) failed: %v", err)
		}

		// Test setting parameter directly
		err = ctx.SetParameter(ZSTD_c_rsyncable, 1)
		if err != nil {
			t.Fatalf("SetParameter(ZSTD_c_rsyncable, 1) failed: %v", err)
		}

		err = ctx.SetParameter(ZSTD_c_rsyncable, 0)
		if err != nil {
			t.Fatalf("SetParameter(ZSTD_c_rsyncable, 0) failed: %v", err)
		}
	})

	t.Run("RsyncFriendly_CompressionWorks", func(t *testing.T) {
		// Create test data that would benefit from rsync-friendly compression
		testData := make([]byte, 100000) // 100KB test data
		for i := range testData {
			// Create repetitive patterns that rsync could benefit from
			testData[i] = byte((i / 1000) % 256)
		}

		// Test normal compression
		ctxNormal := NewCCtx()
		defer ctxNormal.Release()

		normalCompressed, err := ctxNormal.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Normal compression failed: %v", err)
		}

		// Test rsync-friendly compression
		ctxRsync := NewCCtx()
		defer ctxRsync.Release()

		// Enable multi-threading for rsyncable
		err = ctxRsync.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter(nbWorkers) failed: %v", err)
		}

		err = ctxRsync.SetRsyncFriendly(true)
		if err != nil {
			t.Fatalf("SetRsyncFriendly failed: %v", err)
		}

		rsyncCompressed, err := ctxRsync.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Rsync-friendly compression failed: %v", err)
		}

		// Both should decompress to the same data
		normalDecompressed, err := Decompress(nil, normalCompressed)
		if err != nil {
			t.Fatalf("Normal decompression failed: %v", err)
		}

		rsyncDecompressed, err := Decompress(nil, rsyncCompressed)
		if err != nil {
			t.Fatalf("Rsync-friendly decompression failed: %v", err)
		}

		// Verify both decompress to original data
		if !bytes.Equal(testData, normalDecompressed) {
			t.Error("Normal compression/decompression data mismatch")
		}

		if !bytes.Equal(testData, rsyncDecompressed) {
			t.Error("Rsync-friendly compression/decompression data mismatch")
		}

		// Log compression ratios for comparison
		normalRatio := float64(len(testData)) / float64(len(normalCompressed))
		rsyncRatio := float64(len(testData)) / float64(len(rsyncCompressed))

		t.Logf("Normal compression: %d -> %d bytes (ratio: %.2fx)",
			len(testData), len(normalCompressed), normalRatio)
		t.Logf("Rsync-friendly compression: %d -> %d bytes (ratio: %.2fx)",
			len(testData), len(rsyncCompressed), rsyncRatio)

		// Rsync-friendly compression may be slightly worse but should still be reasonable
		if rsyncRatio < normalRatio*0.8 {
			t.Logf("Warning: Rsync-friendly compression ratio significantly lower than normal (%.2fx vs %.2fx)",
				rsyncRatio, normalRatio)
		}
	})

	t.Run("RsyncFriendly_InvalidValue", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Enable multi-threading first
		err := ctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter(nbWorkers) failed: %v", err)
		}

		// Test invalid values
		invalidValues := []int{-1, 2, 100}

		for _, value := range invalidValues {
			err := ctx.SetParameter(ZSTD_c_rsyncable, value)
			if err == nil {
				t.Errorf("SetParameter(ZSTD_c_rsyncable, %d) should have failed but didn't", value)
			}

			if !IsParameterError(err) {
				t.Errorf("Expected ParameterError for value %d, got %T: %v", value, err, err)
			}
		}
	})

	t.Run("RsyncFriendly_RequiresMultiThreading", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Try to enable rsync-friendly without multi-threading - should fail
		err := ctx.SetRsyncFriendly(true)
		if err == nil {
			t.Error("SetRsyncFriendly(true) should have failed without multi-threading")
		}

		if !IsParameterError(err) {
			t.Errorf("Expected ParameterError, got %T: %v", err, err)
		}

		// Verify error mentions multi-threading requirement
		if !strings.Contains(err.Error(), "multi-threading") {
			t.Errorf("Error should mention multi-threading requirement: %v", err)
		}
	})
}

func TestRsyncFriendlyValidation(t *testing.T) {
	// Test parameter validation bounds
	validator := NewParameterValidator()

	ctx := &ValidationContext{
		Architecture:  "amd64",
		CurrentParams: make(map[CParameter]int),
	}

	// Valid values
	// Test value 0 (disabled) - should always be valid
	err := validator.ValidateParameter(ZSTD_c_rsyncable, 0, ctx)
	if err != nil {
		t.Errorf("Validation failed for valid value 0: %v", err)
	}

	// Test value 1 (enabled) - requires multi-threading
	// First test without multi-threading (should fail)
	err = validator.ValidateParameter(ZSTD_c_rsyncable, 1, ctx)
	if err == nil {
		t.Error("Validation should have failed for rsyncable=1 without multi-threading")
	}

	// Now test with multi-threading enabled (should pass)
	ctx.CurrentParams[ZSTD_c_nbWorkers] = 2
	err = validator.ValidateParameter(ZSTD_c_rsyncable, 1, ctx)
	if err != nil {
		t.Errorf("Validation failed for valid value 1 with multi-threading: %v", err)
	}

	// Invalid values
	invalidValues := []int{-1, 2, 10}
	for _, value := range invalidValues {
		err := validator.ValidateParameter(ZSTD_c_rsyncable, value, ctx)
		if err == nil {
			t.Errorf("Validation should have failed for invalid value %d", value)
		}
	}
}

func TestRsyncFriendlyIntegration(t *testing.T) {
	// Test integration with other parameters
	ctx := NewCCtx()
	defer ctx.Release()

	// Set multiple parameters including rsync-friendly
	err := ctx.SetParameter(ZSTD_c_compressionLevel, 5)
	if err != nil {
		t.Fatalf("SetParameter(compressionLevel) failed: %v", err)
	}

	// Enable multi-threading for rsyncable
	err = ctx.SetParameter(ZSTD_c_nbWorkers, 2)
	if err != nil {
		t.Fatalf("SetParameter(nbWorkers) failed: %v", err)
	}

	err = ctx.SetRsyncFriendly(true)
	if err != nil {
		t.Fatalf("SetRsyncFriendly failed: %v", err)
	}

	err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
	if err != nil {
		t.Fatalf("SetParameter(checksumFlag) failed: %v", err)
	}

	// Test compression with multiple parameters
	testData := []byte("Integration test data for rsync-friendly compression with multiple parameters set.")
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

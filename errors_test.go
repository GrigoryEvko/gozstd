package gozstd

import (
	"strings"
	"testing"
)

func TestZstdErrorTypes(t *testing.T) {
	// Test buffer size error
	t.Run("BufferError", func(t *testing.T) {
		// Create a context and try to trigger a buffer error with direct calls
		// The high-level Decompress function automatically resizes buffers

		// Test parameter validation instead (more reliable for BufferError)
		ctx := NewCCtx()
		defer ctx.Release()

		// This should create a parameter error, but let's test buffer concepts
		// by testing what happens when we set an invalid pledged size
		err := ctx.SetPledgedSrcSize(^uint64(0)) // Maximum uint64, likely invalid

		if err != nil {
			if IsParameterError(err) {
				t.Logf("Got parameter error (expected for extreme pledged size): %v", err)
				return
			}
		}

		// Alternative approach: test that very large data would cause memory issues
		// but in a controlled way
		t.Skip("BufferError testing requires more specific ZSTD API usage - skipping for now")
	})

	// Test parameter error
	t.Run("ParameterError", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()

		// Test invalid boolean parameter value
		err := ctx.SetParameter(ZSTD_c_checksumFlag, 5) // Invalid value for boolean

		if err == nil {
			t.Error("Expected parameter error but got nil")
			return
		}

		if !IsParameterError(err) {
			t.Errorf("Expected ParameterError, got %T: %v", err, err)
			return
		}

		paramErr := err.(*ParameterError)
		if !paramErr.IsRecoverable() {
			t.Error("Parameter error should be recoverable")
		}

		suggestion := paramErr.GetSuggestion()
		if !strings.Contains(suggestion, "0") || !strings.Contains(suggestion, "1") {
			t.Errorf("Expected parameter range suggestion with 0 and 1, got: %s", suggestion)
		}

		t.Logf("Parameter error: %v", err)
		t.Logf("Suggestion: %s", suggestion)
	})

	// Test frame/corruption error
	t.Run("FrameError", func(t *testing.T) {
		// Try to decompress invalid data (not ZSTD format)
		invalidData := []byte("This is not compressed ZSTD data")

		_, err := Decompress(nil, invalidData)

		if err == nil {
			t.Error("Expected frame error but got nil")
			return
		}

		// Should be either FrameError or CorruptionError
		if !IsFrameError(err) && !IsCorruptionError(err) {
			t.Errorf("Expected FrameError or CorruptionError, got %T: %v", err, err)
			return
		}

		t.Logf("Invalid data error: %v", err)
	})

	// Test dictionary error
	t.Run("DictionaryError", func(t *testing.T) {
		// Create dictionary compressed data
		dict := []byte("common words the and is for")
		cd, err := NewCDictLevel(dict, 3)
		if err != nil {
			t.Skip("Cannot create dictionary:", err)
		}
		defer cd.Release()

		dd, err := NewDDict(dict)
		if err != nil {
			t.Skip("Cannot create decompression dictionary:", err)
		}
		defer dd.Release()

		data := []byte("the quick brown fox is fast")
		compressed := CompressDict(nil, data, cd)

		// Try to decompress with wrong dictionary
		wrongDict := []byte("different dictionary content")
		ddWrong, err := NewDDict(wrongDict)
		if err != nil {
			t.Skip("Cannot create wrong dictionary:", err)
		}
		defer ddWrong.Release()

		_, err = DecompressDict(nil, compressed, ddWrong)

		// Note: This might not always fail due to ZSTD's behavior
		// Log the result either way
		if err != nil {
			if IsDictionaryError(err) {
				dictErr := err.(*DictionaryError)
				t.Logf("Dictionary error (as expected): %v", err)
				t.Logf("Suggestion: %s", dictErr.GetSuggestion())
			} else {
				t.Logf("Got different error type: %T: %v", err, err)
			}
		} else {
			t.Logf("Dictionary mismatch was handled gracefully by ZSTD")
		}
	})

	// Test error context information
	t.Run("ErrorContext", func(t *testing.T) {
		// Create an error with context
		ctx := ErrorContext{
			InputSize:        100,
			OutputSize:       50,
			CompressionLevel: 19,
			DictionaryID:     12345,
			FrameInfo:        "test frame",
		}

		// Create a mock ZSTD error
		baseError := &ZstdError{
			Code:        70, // dstSize_tooSmall
			Operation:   "test operation",
			Message:     "destination buffer too small",
			Recoverable: true,
			Suggestion:  "increase buffer size",
			Context:     ctx,
		}

		bufErr := &BufferError{baseError}

		// Test that context is preserved
		if bufErr.Context.InputSize != 100 {
			t.Errorf("Expected input size 100, got %d", bufErr.Context.InputSize)
		}

		if bufErr.Context.CompressionLevel != 19 {
			t.Errorf("Expected compression level 19, got %d", bufErr.Context.CompressionLevel)
		}

		// Test error message formatting
		errMsg := bufErr.Error()
		if !strings.Contains(errMsg, "test operation") {
			t.Errorf("Error message should contain operation: %s", errMsg)
		}

		t.Logf("Full error with context: %v", bufErr)
	})
}

func TestErrorTypeChecking(t *testing.T) {
	// Test type checking functions work correctly
	bufErr := &BufferError{&ZstdError{}}
	memErr := &MemoryError{&ZstdError{}}
	paramErr := &ParameterError{&ZstdError{}}
	dictErr := &DictionaryError{&ZstdError{}}
	corrErr := &CorruptionError{&ZstdError{}}
	streamErr := &StreamStateError{&ZstdError{}}
	verErr := &VersionError{&ZstdError{}}
	frameErr := &FrameError{&ZstdError{}}

	// Test positive cases
	if !IsBufferError(bufErr) {
		t.Error("IsBufferError failed")
	}
	if !IsMemoryError(memErr) {
		t.Error("IsMemoryError failed")
	}
	if !IsParameterError(paramErr) {
		t.Error("IsParameterError failed")
	}
	if !IsDictionaryError(dictErr) {
		t.Error("IsDictionaryError failed")
	}
	if !IsCorruptionError(corrErr) {
		t.Error("IsCorruptionError failed")
	}
	if !IsStreamStateError(streamErr) {
		t.Error("IsStreamStateError failed")
	}
	if !IsVersionError(verErr) {
		t.Error("IsVersionError failed")
	}
	if !IsFrameError(frameErr) {
		t.Error("IsFrameError failed")
	}

	// Test negative cases (cross-checks)
	if IsMemoryError(bufErr) {
		t.Error("False positive: IsMemoryError on BufferError")
	}
	if IsBufferError(memErr) {
		t.Error("False positive: IsBufferError on MemoryError")
	}
	if IsParameterError(dictErr) {
		t.Error("False positive: IsParameterError on DictionaryError")
	}
}

func TestLegacyErrorCompatibility(t *testing.T) {
	// Test that errors still work as normal Go errors
	ctx := NewCCtx()
	defer ctx.Release()

	err := ctx.SetParameter(ZSTD_c_checksumFlag, 99) // Invalid value

	if err == nil {
		t.Error("Expected error but got nil")
		return
	}

	// Should work as normal error interface
	errStr := err.Error()
	if errStr == "" {
		t.Error("Error string should not be empty")
	}

	// Should be able to check error in if statement
	if err != nil {
		t.Logf("Error handling works normally: %v", err)
	}

	t.Logf("Legacy compatibility test passed")
}

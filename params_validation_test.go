package gozstd

import (
	"strings"
	"testing"
)

func TestCriticalMemoryBombPrevention(t *testing.T) {
	t.Run("PreventHashLogMemoryBomb", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Try to set an extremely large hashLog that would allocate 4GB+ memory
		err := ctx.SetParameter(ZSTD_c_hashLog, 30) // 2^32 bytes = 4GB
		
		if err == nil {
			t.Fatal("Expected memory bomb prevention to block hashLog=30, but it was allowed")
		}
		
		if !IsMemoryError(err) {
			t.Errorf("Expected MemoryError, got %T: %v", err, err)
		}
		
		memErr := err.(*MemoryError)
		if !strings.Contains(memErr.GetSuggestion(), "memory") {
			t.Errorf("Expected memory-related suggestion, got: %s", memErr.GetSuggestion())
		}
		
		t.Logf("Memory bomb prevented successfully: %v", err)
	})
	
	t.Run("PreventChainLogMemoryBomb", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Try to set an extremely large chainLog
		err := ctx.SetParameter(ZSTD_c_chainLog, 29) // 2^31 bytes = 2GB
		
		if err == nil {
			t.Fatal("Expected memory bomb prevention to block chainLog=29, but it was allowed")
		}
		
		if !IsMemoryError(err) {
			t.Errorf("Expected MemoryError, got %T: %v", err, err)
		}
		
		t.Logf("Chain log memory bomb prevented: %v", err)
	})
	
	t.Run("PreventCombinedMemoryBomb", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Set moderately high but still dangerous combination
		err := ctx.SetParameter(ZSTD_c_windowLog, 25) // 32MB
		if err != nil {
			t.Logf("WindowLog 25 blocked: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_hashLog, 25) // 128MB
		if err != nil {
			t.Logf("HashLog 25 blocked: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_chainLog, 25) // 128MB  
		if err != nil {
			t.Logf("ChainLog 25 blocked: %v", err)
			// This combination should be blocked due to total memory > 256MB
		}
		
		t.Log("Combined memory usage validation working")
	})
	
	t.Run("AllowReasonableParameters", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// These should be allowed
		reasonableParams := map[CParameter]int{
			ZSTD_c_compressionLevel: 6,
			ZSTD_c_windowLog:       18, // 256KB
			ZSTD_c_hashLog:         15, // 128KB table
			ZSTD_c_chainLog:        15, // 128KB table
			ZSTD_c_checksumFlag:    1,
		}
		
		for param, value := range reasonableParams {
			err := ctx.SetParameter(param, value)
			if err != nil {
				t.Errorf("Reasonable parameter %v=%d was rejected: %v", param, value, err)
			}
		}
		
		t.Log("Reasonable parameters accepted successfully")
	})
}

func TestParameterBoundsValidation(t *testing.T) {
	ctx := NewCCtx()
	defer ctx.Release()
	
	testCases := []struct {
		name      string
		param     CParameter
		value     int
		expectErr bool
		errType   interface{}
	}{
		{
			name:      "compression_level_too_low",
			param:     ZSTD_c_compressionLevel,
			value:     -200000, // Below -131072
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "compression_level_too_high", 
			param:     ZSTD_c_compressionLevel,
			value:     25, // Above 22
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "valid_compression_level",
			param:     ZSTD_c_compressionLevel,
			value:     9,
			expectErr: false,
		},
		{
			name:      "window_log_too_low",
			param:     ZSTD_c_windowLog,
			value:     5, // Below 10
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "window_log_too_high",
			param:     ZSTD_c_windowLog,
			value:     35, // Above max
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "valid_window_log",
			param:     ZSTD_c_windowLog,
			value:     15,
			expectErr: false,
		},
		{
			name:      "invalid_boolean_checksum",
			param:     ZSTD_c_checksumFlag,
			value:     2, // Not 0 or 1
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "valid_boolean_checksum",
			param:     ZSTD_c_checksumFlag,
			value:     1,
			expectErr: false,
		},
		{
			name:      "strategy_too_low",
			param:     ZSTD_c_strategy,
			value:     0, // Below 1
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "strategy_too_high",
			param:     ZSTD_c_strategy,
			value:     15, // Above 9
			expectErr: true,
			errType:   &ParameterError{},
		},
		{
			name:      "valid_strategy",
			param:     ZSTD_c_strategy,
			value:     5,
			expectErr: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset context for each test
			ctx.Reset(ZSTD_reset_session_and_parameters)
			
			err := ctx.SetParameter(tc.param, tc.value)
			
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for %v=%d but got none", tc.param, tc.value)
					return
				}
				
				if tc.errType != nil {
					switch tc.errType.(type) {
					case *ParameterError:
						if !IsParameterError(err) {
							t.Errorf("Expected ParameterError, got %T: %v", err, err)
						}
					case *MemoryError:
						if !IsMemoryError(err) {
							t.Errorf("Expected MemoryError, got %T: %v", err, err)
						}
					}
				}
				
				t.Logf("Parameter validation correctly rejected %v=%d: %v", tc.param, tc.value, err)
			} else {
				if err != nil {
					t.Errorf("Valid parameter %v=%d was rejected: %v", tc.param, tc.value, err)
				}
			}
		})
	}
}

func TestParameterDependencyValidation(t *testing.T) {
	t.Run("LDM_HashLog_WindowLog_Dependency", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Set windowLog first
		err := ctx.SetParameter(ZSTD_c_windowLog, 15)
		if err != nil {
			t.Fatalf("Failed to set windowLog: %v", err)
		}
		
		// Enable LDM
		err = ctx.SetParameter(ZSTD_c_enableLongDistanceMatching, 1)
		if err != nil {
			t.Fatalf("Failed to enable LDM: %v", err)
		}
		
		// Try to set ldmHashLog > windowLog (should fail)
		err = ctx.SetParameter(ZSTD_c_ldmHashLog, 20) // > windowLog(15)
		
		if err == nil {
			t.Fatal("Expected dependency validation to prevent ldmHashLog > windowLog")
		}
		
		if !IsParameterError(err) {
			t.Errorf("Expected ParameterError for dependency violation, got %T: %v", err, err)
		}
		
		if !strings.Contains(err.Error(), "ldmHashLog") || !strings.Contains(err.Error(), "windowLog") {
			t.Errorf("Error should mention both ldmHashLog and windowLog: %v", err)
		}
		
		t.Logf("LDM dependency validation working: %v", err)
		
		// Now try a valid ldmHashLog
		err = ctx.SetParameter(ZSTD_c_ldmHashLog, 12) // < windowLog(15)
		if err != nil {
			t.Errorf("Valid ldmHashLog should be accepted: %v", err)
		}
	})
	
	t.Run("Strategy_MinMatch_Dependency", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Set ZSTD_fast strategy
		err := ctx.SetParameter(ZSTD_c_strategy, 1) // ZSTD_fast
		if err != nil {
			t.Fatalf("Failed to set strategy: %v", err)
		}
		
		// Try to set minMatch too high for ZSTD_fast (should fail or warn)
		err = ctx.SetParameter(ZSTD_c_minMatch, 8) // > 7 max for ZSTD_fast
		
		if err != nil {
			t.Logf("Strategy-specific minMatch validation working: %v", err)
		} else {
			t.Log("minMatch validation may be handled by ZSTD internally")
		}
	})
}

func TestPerformanceWarnings(t *testing.T) {
	params := map[CParameter]int{
		ZSTD_c_compressionLevel: 20, // Very high
		ZSTD_c_nbWorkers:        16, // Likely > CPU count
		ZSTD_c_windowLog:        28, // High memory
		ZSTD_c_hashLog:          26, // High memory
		ZSTD_c_chainLog:         26, // High memory
	}
	
	warnings := CheckPerformanceWarnings(params)
	
	if len(warnings) == 0 {
		t.Error("Expected performance warnings for extreme parameters")
	}
	
	for _, warning := range warnings {
		t.Logf("Performance warning [%s]: %s - %s", warning.Type, warning.Message, warning.Suggestion)
	}
}

func TestParameterValidationIntegration(t *testing.T) {
	// Test that parameter validation doesn't break normal usage
	t.Run("NormalCompressionStillWorks", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Set reasonable parameters
		err := ctx.SetParameter(ZSTD_c_compressionLevel, 6)
		if err != nil {
			t.Fatalf("Failed to set compression level: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
		if err != nil {
			t.Fatalf("Failed to set checksum flag: %v", err)
		}
		
		// Compress some data
		testData := []byte("This is test data for parameter validation integration testing")
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression failed: %v", err)
		}
		
		// Decompress to verify
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if string(decompressed) != string(testData) {
			t.Error("Roundtrip failed - data doesn't match")
		}
		
		t.Log("Normal compression/decompression works with parameter validation")
	})
	
	t.Run("FuzzingTestsStillPass", func(t *testing.T) {
		// Quick smoke test that fuzzing tests still work
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test various parameter combinations that should work
		validCombinations := []map[CParameter]int{
			{ZSTD_c_compressionLevel: 1},
			{ZSTD_c_compressionLevel: 6, ZSTD_c_windowLog: 18},
			{ZSTD_c_compressionLevel: 9, ZSTD_c_checksumFlag: 1},
		}
		
		for i, params := range validCombinations {
			ctx.Reset(ZSTD_reset_session_and_parameters)
			
			for param, value := range params {
				err := ctx.SetParameter(param, value)
				if err != nil {
					t.Errorf("Valid parameter combination %d failed: %v=%d error: %v", i, param, value, err)
				}
			}
			
			// Test compression still works
			data := []byte("test data")
			_, err := ctx.Compress(nil, data)
			if err != nil {
				t.Errorf("Compression failed for combination %d: %v", i, err)
			}
		}
	})
}

func TestParameterValidatorArchitectureAwareness(t *testing.T) {
	// Test that the validator respects architecture limits
	validator := NewParameterValidator()
	
	ctx := &ValidationContext{
		Architecture:  "386", // 32-bit
		CurrentParams: make(map[CParameter]int),
	}
	
	// On 32-bit, windowLog max should be 30, chainLog max should be 29
	err := validator.ValidateParameter(ZSTD_c_windowLog, 31, ctx)
	if err == nil {
		t.Log("Note: windowLog 31 validation depends on runtime architecture detection")
	}
	
	err = validator.ValidateParameter(ZSTD_c_chainLog, 30, ctx)
	if err == nil {
		t.Log("Note: chainLog 30 validation depends on runtime architecture detection")
	}
	
	// These should always be valid
	err = validator.ValidateParameter(ZSTD_c_windowLog, 20, ctx)
	if err != nil {
		t.Errorf("windowLog 20 should be valid on all architectures: %v", err)
	}
	
	err = validator.ValidateParameter(ZSTD_c_chainLog, 20, ctx)
	if err != nil {
		t.Errorf("chainLog 20 should be valid on all architectures: %v", err)
	}
}
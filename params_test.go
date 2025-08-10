package gozstd

import (
	"math"
	"runtime"
	"testing"
)

// FuzzParameterBoundaries tests extreme parameter values
func FuzzParameterBoundaries(f *testing.F) {
	// Test with extreme values
	f.Add([]byte("test"), -2147483648, 10, 10, 10, 10, 10, 10, 0) // min int32
	f.Add([]byte("test"), 2147483647, 10, 10, 10, 10, 10, 10, 0)  // max int32
	f.Add([]byte("test"), 3, 0, 0, 0, 0, 0, 0, 0)                 // all zeros
	f.Add([]byte("test"), 3, 64, 64, 64, 64, 64, 64, 999)         // large values

	f.Fuzz(func(t *testing.T, data []byte,
		level, windowLog, hashLog, chainLog, searchLog, minMatch, targetLength, strategy int) {

		ctx := NewCCtx()

		// Test setting each parameter with the fuzzed value
		params := []struct {
			param CParameter
			value int
			name  string
		}{
			{ZSTD_c_compressionLevel, level, "compressionLevel"},
			{ZSTD_c_windowLog, windowLog, "windowLog"},
			{ZSTD_c_hashLog, hashLog, "hashLog"},
			{ZSTD_c_chainLog, chainLog, "chainLog"},
			{ZSTD_c_searchLog, searchLog, "searchLog"},
			{ZSTD_c_minMatch, minMatch, "minMatch"},
			{ZSTD_c_targetLength, targetLength, "targetLength"},
			{ZSTD_c_strategy, strategy, "strategy"},
		}

		errors := 0
		for _, p := range params {
			err := ctx.SetParameter(p.param, p.value)
			if err != nil {
				// Some errors are expected for invalid values
				t.Logf("SetParameter(%s, %d) error: %v", p.name, p.value, err)
				errors++
			}
		}

		// If all parameters errored, skip compression test
		if errors == len(params) {
			return
		}

		// Try to compress with these parameters
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Logf("Compression failed with parameters: %v", err)
			return
		}

		// Verify we can decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Failed to decompress data compressed with extreme parameters: %v", err)
			return
		}

		if string(decompressed) != string(data) {
			t.Errorf("Decompression mismatch with extreme parameters")
		}
	})
}

// FuzzConflictingParameters tests invalid parameter combinations
func FuzzConflictingParameters(f *testing.F) {
	f.Add([]byte("test"), 10, 20, 1, 15, 30, 10, 20)

	f.Fuzz(func(t *testing.T, data []byte,
		windowLog, ldmHashLog, enableLdm, hashLog, chainLog, nbWorkers, jobSize int) {

		ctx := NewCCtx()

		// Set potentially conflicting parameters

		// Test 1: LDM constraints
		// ldmHashLog must be <= windowLog
		// ldmHashLog minimum is 6
		ctx.SetParameter(ZSTD_c_windowLog, windowLog)
		ctx.SetParameter(ZSTD_c_enableLongDistanceMatching, enableLdm&1)
		if enableLdm&1 == 1 {
			err := ctx.SetParameter(ZSTD_c_ldmHashLog, ldmHashLog)
			if ldmHashLog > windowLog {
				if err == nil {
					t.Logf("ZSTD allowed ldmHashLog(%d) > windowLog(%d) - may clamp internally", ldmHashLog, windowLog)
				} else {
					t.Logf("ZSTD correctly rejected ldmHashLog(%d) > windowLog(%d): %v", ldmHashLog, windowLog, err)
				}
			}
		}

		// Test 2: Hash chain constraints
		// hashLog + chainLog should be <= 64 (or some limit)
		ctx.SetParameter(ZSTD_c_hashLog, hashLog)
		ctx.SetParameter(ZSTD_c_chainLog, chainLog)

		// Test 3: Multi-threading constraints
		// jobSize should be reasonable when nbWorkers > 1
		if nbWorkers > 1 {
			ctx.SetParameter(ZSTD_c_nbWorkers, nbWorkers)
			ctx.SetParameter(ZSTD_c_jobSize, jobSize)
		}

		// Try compression
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			// Some combinations should fail
			t.Logf("Expected compression failure: %v", err)
			return
		}

		// Verify decompression still works
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Decompression failed after conflicting params: %v", err)
		} else if string(decompressed) != string(data) {
			t.Errorf("Data corruption with conflicting parameters")
		}
	})
}

// FuzzAllParameterCombinations systematically tests all parameters
func FuzzAllParameterCombinations(f *testing.F) {
	// Add seed with various parameter counts
	f.Add([]byte("test"), uint64(0))
	f.Add([]byte("test"), uint64(0xFFFFFFFF))
	f.Add([]byte("test"), uint64(0x123456789ABCDEF))

	f.Fuzz(func(t *testing.T, data []byte, paramBits uint64) {
		ctx := NewCCtx()

		// All available parameters
		allParams := []struct {
			param  CParameter
			minVal int
			maxVal int
			name   string
		}{
			{ZSTD_c_compressionLevel, -131072, 22, "compressionLevel"},
			{ZSTD_c_windowLog, 10, 31, "windowLog"}, // 31 for 64-bit
			{ZSTD_c_hashLog, 6, 30, "hashLog"},
			{ZSTD_c_chainLog, 6, 30, "chainLog"},
			{ZSTD_c_searchLog, 1, 30, "searchLog"},
			{ZSTD_c_minMatch, 3, 7, "minMatch"},
			{ZSTD_c_targetLength, 0, 131072, "targetLength"},
			{ZSTD_c_strategy, 0, 9, "strategy"},
			{ZSTD_c_enableLongDistanceMatching, 0, 1, "enableLDM"},
			{ZSTD_c_ldmHashLog, 6, 30, "ldmHashLog"},
			{ZSTD_c_ldmMinMatch, 4, 4096, "ldmMinMatch"},
			{ZSTD_c_ldmBucketSizeLog, 1, 8, "ldmBucketSizeLog"},
			{ZSTD_c_ldmHashRateLog, 0, 30, "ldmHashRateLog"},
			{ZSTD_c_contentSizeFlag, 0, 1, "contentSizeFlag"},
			{ZSTD_c_checksumFlag, 0, 1, "checksumFlag"},
			{ZSTD_c_dictIDFlag, 0, 1, "dictIDFlag"},
		}

		// Use paramBits to decide which parameters to set
		setCount := 0
		for i, p := range allParams {
			if paramBits&(1<<uint(i)) != 0 {
				// Calculate value based on bits
				valueBits := int((paramBits >> (i * 4)) & 0xF)
				value := p.minVal + (valueBits * (p.maxVal - p.minVal) / 15)

				err := ctx.SetParameter(p.param, value)
				if err != nil {
					t.Logf("SetParameter(%s, %d) failed: %v", p.name, value, err)
				} else {
					setCount++
				}
			}
		}

		if setCount == 0 {
			return // No parameters set
		}

		// Test compression
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Logf("Compression failed with %d parameters set: %v", setCount, err)
			return
		}

		// Verify
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Decompression failed: %v", err)
		} else if string(decompressed) != string(data) {
			t.Errorf("Data mismatch after compression with %d parameters", setCount)
		}
	})
}

// FuzzParameterOverflow tests integer overflow scenarios
func FuzzParameterOverflow(f *testing.F) {
	// Test overflow scenarios
	f.Add([]byte("test"), math.MaxInt32, math.MaxInt32, 1)
	f.Add([]byte("test"), math.MinInt32, math.MinInt32, -1)
	f.Add([]byte("test"), 1<<30, 1<<30, 1<<20)

	f.Fuzz(func(t *testing.T, data []byte, param1, param2, multiplier int) {
		ctx := NewCCtx()

		// Test various overflow scenarios
		testCases := []struct {
			name  string
			param CParameter
			value int
		}{
			// Direct overflow
			{"compressionLevel-overflow", ZSTD_c_compressionLevel, param1},
			{"windowLog-overflow", ZSTD_c_windowLog, param1 & 0x3F}, // Limit to 6 bits
			{"targetLength-overflow", ZSTD_c_targetLength, param1 * multiplier},

			// Calculated overflows
			{"hashLog-calculated", ZSTD_c_hashLog, (param1 + param2) & 0x1F},
			{"chainLog-calculated", ZSTD_c_chainLog, (param1 * param2) & 0x1F},

			// Negative to positive overflow
			{"searchLog-wrapped", ZSTD_c_searchLog, int(uint32(param1))},
		}

		for _, tc := range testCases {
			err := ctx.SetParameter(tc.param, tc.value)
			if err != nil {
				t.Logf("%s: SetParameter(%d) failed as expected: %v", tc.name, tc.value, err)
			} else {
				t.Logf("%s: SetParameter(%d) succeeded", tc.name, tc.value)
			}
		}

		// Try compression regardless
		compressed, err := ctx.Compress(nil, data)
		if err == nil {
			// If it worked, verify correctness
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Errorf("Decompression failed after overflow parameters: %v", err)
			} else if string(decompressed) != string(data) {
				t.Errorf("Data corruption after overflow parameters")
			}
		}
	})
}

// FuzzMultiThreadingParameters tests nbWorkers and jobSize edge cases
func FuzzMultiThreadingParameters(f *testing.F) {
	f.Add([]byte("small"), 0, 0)
	f.Add([]byte("larger data"), 100, 1)
	f.Add(make([]byte, 1<<20), 8, 1<<15)

	f.Fuzz(func(t *testing.T, data []byte, nbWorkers, jobSize int) {
		ctx := NewCCtx()

		// Test extreme nbWorkers values
		// Note: Values significantly exceeding CPU count are now rejected for safety
		maxReasonableWorkers := runtime.NumCPU() * 2
		err := ctx.SetParameter(ZSTD_c_nbWorkers, nbWorkers)
		if err != nil && nbWorkers >= 0 && nbWorkers <= maxReasonableWorkers {
			// This should have worked
			t.Errorf("Failed to set valid nbWorkers=%d: %v", nbWorkers, err)
		}

		// Test extreme jobSize values
		if nbWorkers > 0 {
			err = ctx.SetParameter(ZSTD_c_jobSize, jobSize)
			if err != nil && jobSize > 0 {
				t.Logf("Failed to set jobSize=%d: %v", jobSize, err)
			}

			// Test overlap log
			overlapLog := (jobSize & 0xF) // 0-15
			ctx.SetParameter(ZSTD_c_overlapLog, overlapLog)
		}

		// Compress with multi-threading parameters
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Logf("Compression failed with nbWorkers=%d, jobSize=%d: %v",
				nbWorkers, jobSize, err)
			return
		}

		// Verify correctness
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Errorf("Decompression failed: %v", err)
		} else if string(decompressed) != string(data) {
			t.Errorf("Data mismatch with multi-threading params")
		}

		// Test interaction with other parameters
		ctx.SetParameter(ZSTD_c_compressionLevel, 19) // High compression
		ctx.SetParameter(ZSTD_c_windowLog, 27)        // Large window

		compressed, err = ctx.Compress(nil, data)
		if err == nil {
			decompressed, _ = Decompress(nil, compressed)
			if string(decompressed) != string(data) {
				t.Errorf("Data corruption with MT + high compression")
			}
		}
	})
}

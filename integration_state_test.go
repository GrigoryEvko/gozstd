package gozstd

import (
	"bytes"
	"testing"
)

// FuzzStateMachineReset tests various reset scenarios
func FuzzStateMachineReset(f *testing.F) {
	f.Add([]byte("test"), 3, 0, 1, 2, 0, 1)
	f.Add([]byte("data"), 5, 2, 0, 1, 1, 0)
	
	f.Fuzz(func(t *testing.T, data []byte, level int,
		reset1, reset2, reset3 int, // Reset directives (0-2)
		param1, param2 int) {        // Parameters to set
		
		ctx := NewCCtx()
		
		// Helper to get reset directive
		getResetDirective := func(r int) ZSTD_ResetDirective {
			switch r % 3 {
			case 0:
				return ZSTD_reset_session_only
			case 1:
				return ZSTD_reset_parameters
			default:
				return ZSTD_reset_session_and_parameters
			}
		}
		
		// Initial setup
		ctx.SetParameter(ZSTD_c_compressionLevel, level)
		ctx.SetParameter(ZSTD_c_checksumFlag, param1&1)
		ctx.SetParameter(ZSTD_c_windowLog, 10+(param2&0xF)) // 10-25
		
		// First compression
		compressed1, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Initial compression failed: %v", err)
		}
		
		// Reset 1
		err = ctx.Reset(getResetDirective(reset1))
		if err != nil {
			t.Fatalf("Reset 1 failed: %v", err)
		}
		
		// Try to compress without re-setting parameters
		compressed2, err := ctx.Compress(nil, data)
		if err != nil {
			// This might fail if we reset parameters
			if reset1%3 == 1 { // ZSTD_reset_parameters
				t.Logf("Expected failure after parameter reset: %v", err)
				// Re-set parameters
				ctx.SetParameter(ZSTD_c_compressionLevel, level)
			} else {
				t.Errorf("Unexpected compression failure after reset: %v", err)
			}
		}
		
		// Reset 2 - different type
		err = ctx.Reset(getResetDirective(reset2))
		if err != nil {
			t.Fatalf("Reset 2 failed: %v", err)
		}
		
		// Change some parameters
		ctx.SetParameter(ZSTD_c_compressionLevel, (level+1)%20)
		ctx.SetParameter(ZSTD_c_strategy, param1%9)
		
		// Compress again
		compressed3, err := ctx.Compress(nil, data)
		if err != nil {
			// Re-set all parameters if needed
			ctx.SetParameter(ZSTD_c_compressionLevel, 3)
			compressed3, err = ctx.Compress(nil, data)
			if err != nil {
				t.Fatalf("Compression failed even after re-setting params: %v", err)
			}
		}
		
		// Reset 3 - rapid resets
		for i := 0; i < 5; i++ {
			ctx.Reset(getResetDirective((reset3 + i) % 3))
		}
		
		// Final compression
		ctx.SetParameter(ZSTD_c_compressionLevel, 1)
		compressed4, err := ctx.Compress(nil, data)
		if err != nil {
			t.Errorf("Final compression failed after multiple resets: %v", err)
		}
		
		// Verify all compressed data can be decompressed
		for i, compressed := range [][]byte{compressed1, compressed2, compressed3, compressed4} {
			if len(compressed) == 0 {
				continue
			}
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Errorf("Failed to decompress result %d: %v", i, err)
			} else if !bytes.Equal(decompressed, data) {
				t.Errorf("Decompression mismatch for result %d", i)
			}
		}
	})
}

// FuzzPledgedSizeLies tests mismatched pledged sizes
func FuzzPledgedSizeLies(f *testing.F) {
	f.Add([]byte("exact"), 5, 5)
	f.Add([]byte("short data"), 100, 10)
	f.Add([]byte("very long data that exceeds pledge"), 10, 35)
	f.Add(make([]byte, 1000), 10, 1000)
	
	f.Fuzz(func(t *testing.T, data []byte, pledged int, actual int) {
		if pledged < 0 || actual < 0 || actual > len(data) {
			return
		}
		
		ctx := NewCCtx()
		
		// Set pledged size
		err := ctx.SetPledgedSrcSize(uint64(pledged))
		if err != nil {
			t.Logf("SetPledgedSrcSize(%d) failed: %v", pledged, err)
			return
		}
		
		// Use different actual size
		actualData := data
		if actual < len(data) {
			actualData = data[:actual]
		}
		
		// Try to compress
		compressed, err := ctx.Compress(nil, actualData)
		
		if pledged != len(actualData) {
			// Mismatch - this might fail
			if err != nil {
				t.Logf("Expected error with pledged=%d, actual=%d: %v", 
					pledged, len(actualData), err)
				return
			}
			// If it succeeded, that's interesting
			t.Logf("Compression succeeded despite size mismatch: pledged=%d, actual=%d",
				pledged, len(actualData))
		} else {
			// Sizes match - should succeed
			if err != nil {
				t.Errorf("Compression failed with correct pledged size: %v", err)
				return
			}
		}
		
		// Verify decompression
		if len(compressed) > 0 {
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Errorf("Decompression failed: %v", err)
			} else if !bytes.Equal(decompressed, actualData) {
				t.Errorf("Data mismatch after pledged size compression")
			}
		}
		
		// Test ZSTD_CONTENTSIZE_UNKNOWN (0)
		ctx.Reset(ZSTD_reset_session_only)
		err = ctx.SetPledgedSrcSize(0) // 0 means empty, not unknown!
		if err == nil {
			compressed, err = ctx.Compress(nil, data)
			if err == nil && len(data) > 0 {
				// This should have failed - we pledged 0 but sent data
				t.Logf("Compressed %d bytes after pledging 0", len(data))
			}
		}
	})
}

// FuzzContextReuse tests reusing contexts for multiple operations
func FuzzContextReuse(f *testing.F) {
	f.Add([]byte("first"), []byte("second"), []byte("third"), 3, 5, 7)
	
	f.Fuzz(func(t *testing.T, data1, data2, data3 []byte, 
		level1, level2, level3 int) {
		
		ctx := NewCCtx()
		var compressed [][]byte
		
		// Use same context for multiple compressions with different settings
		datasets := []struct {
			data  []byte
			level int
			flags int
		}{
			{data1, level1, 0},
			{data2, level2, 1}, // Enable checksum
			{data3, level3, 0},
		}
		
		for i, ds := range datasets {
			// Change parameters each time
			ctx.SetParameter(ZSTD_c_compressionLevel, ds.level)
			ctx.SetParameter(ZSTD_c_checksumFlag, ds.flags)
			
			// Also change strategy
			ctx.SetParameter(ZSTD_c_strategy, i%9)
			
			// Compress
			result, err := ctx.Compress(nil, ds.data)
			if err != nil {
				t.Errorf("Compression %d failed: %v", i, err)
				continue
			}
			compressed = append(compressed, result)
			
			// Don't reset between compressions - test state leakage
		}
		
		// Verify all can be decompressed correctly
		for i, comp := range compressed {
			decompressed, err := Decompress(nil, comp)
			if err != nil {
				t.Errorf("Failed to decompress result %d: %v", i, err)
				continue
			}
			
			expected := datasets[i].data
			if !bytes.Equal(decompressed, expected) {
				t.Errorf("Data mismatch for compression %d", i)
			}
		}
		
		// Now test with resets between compressions
		compressed = compressed[:0]
		for i, ds := range datasets {
			if i > 0 {
				// Reset session only - parameters should remain
				ctx.Reset(ZSTD_reset_session_only)
			}
			
			ctx.SetParameter(ZSTD_c_compressionLevel, ds.level)
			result, err := ctx.Compress(nil, ds.data)
			if err != nil {
				t.Errorf("Compression %d with reset failed: %v", i, err)
				continue
			}
			compressed = append(compressed, result)
		}
		
		// Verify again
		for i, comp := range compressed {
			decompressed, err := Decompress(nil, comp)
			if err != nil {
				t.Errorf("Failed to decompress reset result %d: %v", i, err)
			} else if !bytes.Equal(decompressed, datasets[i%len(datasets)].data) {
				t.Errorf("Data mismatch for reset compression %d", i)
			}
		}
	})
}

// FuzzPoolContamination tests putting modified contexts back in pool
func FuzzPoolContamination(f *testing.F) {
	f.Add([]byte("test"), 3, 1, 15)
	
	f.Fuzz(func(t *testing.T, data []byte, level int, checksum int, windowLog int) {
		// The current implementation has a bug - NewCCtx() takes from pool
		// but never returns it. Let's test the regular Compress functions
		// which do use the pool correctly
		
		// First compression with specific settings
		compressed1 := CompressLevel(nil, data, level)
		
		// The context used is now back in the pool
		// Let's do another compression and see if settings leaked
		compressed2 := CompressLevel(nil, data, DefaultCompressionLevel)
		
		// Both should decompress correctly
		dec1, err1 := Decompress(nil, compressed1)
		dec2, err2 := Decompress(nil, compressed2)
		
		if err1 != nil || err2 != nil {
			t.Errorf("Decompression errors: %v, %v", err1, err2)
			return
		}
		
		if !bytes.Equal(dec1, data) || !bytes.Equal(dec2, data) {
			t.Error("Data corruption from pool contamination")
		}
		
		// Test dictionary pool contamination
		dict := []byte("dictionary content for testing pool contamination")
		cd, err := NewCDict(dict)
		if err != nil {
			return
		}
		defer cd.Release()
		
		// Compress with dictionary
		compressedDict := CompressDict(nil, data, cd)
		
		// Compress without dictionary - should not be affected
		compressedNoDict := Compress(nil, data)
		
		// Try to decompress dict-compressed data without dictionary
		// According to ZSTD: only fails if frame specifies dictionary ID
		_, err = Decompress(nil, compressedDict)
		if err == nil {
			t.Logf("Decompression without dictionary succeeded (frame may not require dictionary)")
		} else {
			t.Logf("Decompression without dictionary failed as expected: %v", err)
		}
		
		// Regular compressed data should work
		dec, err := Decompress(nil, compressedNoDict)
		if err != nil {
			t.Errorf("Failed to decompress non-dict data: %v", err)
		} else if !bytes.Equal(dec, data) {
			t.Error("Pool contamination affected non-dict compression")
		}
	})
}

// FuzzRapidContextOperations tests rapid state changes
func FuzzRapidContextOperations(f *testing.F) {
	f.Add([]byte("rapid test"), uint32(0x12345678))
	
	f.Fuzz(func(t *testing.T, data []byte, operations uint32) {
		ctx := NewCCtx()
		
		// Use operations bits to determine what to do
		for i := 0; i < 32; i++ {
			op := (operations >> (i * 2)) & 0x3
			
			switch op {
			case 0: // Set parameter
				param := (operations >> i) & 0xF
				value := int(operations>>(i+4)) & 0xFF
				
				// Set various parameters based on bits
				switch param % 5 {
				case 0:
					ctx.SetParameter(ZSTD_c_compressionLevel, value%23)
				case 1:
					ctx.SetParameter(ZSTD_c_windowLog, 10+(value%18))
				case 2:
					ctx.SetParameter(ZSTD_c_strategy, value%9)
				case 3:
					ctx.SetParameter(ZSTD_c_checksumFlag, value&1)
				case 4:
					ctx.SetParameter(ZSTD_c_contentSizeFlag, value&1)
				}
				
			case 1: // Reset
				resetType := i % 3
				var directive ZSTD_ResetDirective
				switch resetType {
				case 0:
					directive = ZSTD_reset_session_only
				case 1:
					directive = ZSTD_reset_parameters
				case 2:
					directive = ZSTD_reset_session_and_parameters
				}
				ctx.Reset(directive)
				
			case 2: // Compress
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					// After parameter reset, we need to set level again
					ctx.SetParameter(ZSTD_c_compressionLevel, 3)
					compressed, _ = ctx.Compress(nil, data)
				}
				
				// Verify if we got data
				if len(compressed) > 0 {
					decompressed, err := Decompress(nil, compressed)
					if err != nil {
						t.Logf("Decompression failed after rapid ops: %v", err)
					} else if !bytes.Equal(decompressed, data) {
						t.Error("Data corruption after rapid operations")
					}
				}
				
			case 3: // Set pledged size
				size := uint64(operations>>(i*3)) & 0xFFFF
				ctx.SetPledgedSrcSize(size)
			}
		}
		
		// Final compression test
		ctx.SetParameter(ZSTD_c_compressionLevel, 5)
		compressed, err := ctx.Compress(nil, data)
		if err == nil {
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Errorf("Final decompression failed: %v", err)
			} else if !bytes.Equal(decompressed, data) {
				t.Error("Final data corruption")
			}
		}
	})
}
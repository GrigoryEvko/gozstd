package gozstd

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// FuzzBitFlipping systematically flips bits in compressed data
func FuzzBitFlipping(f *testing.F) {
	// Create various types of compressed data
	validCompressed := [][]byte{
		Compress(nil, []byte("simple")),
		CompressLevel(nil, []byte("with levels"), 19),
		CompressLevel(nil, bytes.Repeat([]byte("repeated "), 100), 1),
	}
	
	for _, compressed := range validCompressed {
		for i := 0; i < len(compressed) && i < 20; i++ {
			f.Add(compressed, i, uint8(1<<3)) // Flip different bits
		}
	}
	
	f.Fuzz(func(t *testing.T, compressed []byte, position int, bitMask uint8) {
		if len(compressed) == 0 {
			return
		}
		
		// Make a copy to corrupt
		corrupted := make([]byte, len(compressed))
		copy(corrupted, compressed)
		
		// Flip bits at position
		if position >= 0 && position < len(corrupted) {
			corrupted[position] ^= bitMask
		}
		
		// Try to decompress
		result, err := Decompress(nil, corrupted)
		
		if err == nil {
			// Successful decompression of corrupted data is concerning
			// Let's verify it matches something reasonable
			if len(result) > 10*len(compressed) {
				t.Errorf("Decompressed corrupted data expanded too much: %d -> %d bytes",
					len(compressed), len(result))
			}
			
			// Try to compress the result and see if it's valid
			recompressed := Compress(nil, result)
			if len(recompressed) == 0 {
				t.Error("Recompression of corrupted decompression failed")
			}
		} else {
			// Expected - corruption should usually cause errors
			t.Logf("Expected error on corrupted data: %v", err)
		}
		
		// Test multiple bit flips
		for i := 0; i < 8; i++ {
			if position+i < len(corrupted) {
				corrupted[position+i] ^= (1 << (i % 8))
			}
		}
		
		// This should definitely fail
		_, err = Decompress(nil, corrupted)
		if err == nil {
			t.Error("Multiple bit flips should have caused decompression error")
		}
	})
}

// FuzzTruncation tests with truncated compressed data
func FuzzTruncation(f *testing.F) {
	data := []byte("This is test data for truncation fuzzing")
	compressed := Compress(nil, data)
	
	// Test truncating at various positions
	for i := 0; i <= len(compressed); i += len(compressed) / 10 {
		f.Add(compressed, i)
	}
	
	f.Fuzz(func(t *testing.T, compressed []byte, truncateAt int) {
		if len(compressed) == 0 {
			return
		}
		
		// Truncate at position
		if truncateAt < 0 {
			truncateAt = 0
		}
		if truncateAt > len(compressed) {
			truncateAt = len(compressed)
		}
		
		truncated := compressed[:truncateAt]
		
		// Try to decompress truncated data
		result, err := Decompress(nil, truncated)
		
		if truncateAt < 4 {
			// Less than magic number - must fail
			if err == nil {
				t.Errorf("Decompression succeeded with only %d bytes", truncateAt)
			}
		} else if truncateAt < len(compressed) {
			// Partially truncated - should usually fail
			if err == nil {
				t.Logf("Decompression succeeded with %d/%d bytes, result: %d bytes",
					truncateAt, len(compressed), len(result))
				
				// Verify the partial result is reasonable
				if len(result) > 100*len(compressed) {
					t.Error("Truncated decompression produced unreasonable output")
				}
			}
		} else {
			// Full data - should succeed
			if err != nil {
				t.Errorf("Full decompression failed: %v", err)
			}
		}
		
		// Test progressive truncation
		for i := len(compressed); i > 0; i-- {
			_, err := Decompress(nil, compressed[:i])
			if err == nil && i < len(compressed)-10 {
				// Interesting if it works when significantly truncated
				t.Logf("Decompression worked with %d/%d bytes", i, len(compressed))
				break
			}
		}
	})
}

// FuzzHeaderCorruption targets ZSTD frame headers
func FuzzHeaderCorruption(f *testing.F) {
	// Create different types of frames
	frames := [][]byte{
		Compress(nil, []byte("simple")),
		func() []byte {
			ctx := NewCCtx()
			ctx.SetParameter(ZSTD_c_checksumFlag, 1)
			ctx.SetParameter(ZSTD_c_contentSizeFlag, 1)
			result, _ := ctx.Compress(nil, []byte("with flags"))
			return result
		}(),
		func() []byte {
			ctx := NewCCtx()
			ctx.SetParameter(ZSTD_c_windowLog, 20)
			result, _ := ctx.Compress(nil, bytes.Repeat([]byte("large "), 1000))
			return result
		}(),
	}
	
	for _, frame := range frames {
		f.Add(frame, 0, uint32(0x12345678))
		f.Add(frame, 4, uint32(0xFFFFFFFF))
	}
	
	f.Fuzz(func(t *testing.T, compressed []byte, offset int, corruptValue uint32) {
		if len(compressed) < 8 {
			return
		}
		
		corrupted := make([]byte, len(compressed))
		copy(corrupted, compressed)
		
		// ZSTD frame format:
		// - Magic Number (4 bytes): 0xFD2FB528
		// - Frame Header (2-14 bytes)
		// - Data Blocks
		// - Optional Checksum (4 bytes)
		
		// Corrupt magic number
		if offset == 0 {
			binary.LittleEndian.PutUint32(corrupted[0:4], corruptValue)
			_, err := Decompress(nil, corrupted)
			if err == nil {
				t.Error("Decompression succeeded with corrupted magic number")
			}
			return
		}
		
		// Corrupt frame header descriptor
		if offset == 4 && len(corrupted) > 4 {
			corrupted[4] = byte(corruptValue)
			
			// Extract frame header descriptor bits
			fhd := corrupted[4]
			frameContentSizeFlag := (fhd >> 6) & 0x3
			singleSegmentFlag := (fhd >> 5) & 0x1
			checksumFlag := (fhd >> 2) & 0x1
			dictIDFlag := fhd & 0x3
			
			t.Logf("Corrupted FHD: FCSFlag=%d, SingleSeg=%d, Checksum=%d, DictID=%d",
				frameContentSizeFlag, singleSegmentFlag, checksumFlag, dictIDFlag)
			
			_, err := Decompress(nil, corrupted)
			if err != nil {
				t.Logf("Expected error with corrupted frame header: %v", err)
			} else {
				t.Log("Decompression succeeded despite corrupted frame header")
			}
		}
		
		// Corrupt size fields
		if offset > 4 && offset < len(corrupted)-4 {
			// Inject large size value
			if offset+4 <= len(corrupted) {
				binary.LittleEndian.PutUint32(corrupted[offset:offset+4], corruptValue)
			}
			
			_, err := Decompress(nil, corrupted)
			if err != nil {
				t.Logf("Expected error with corrupted size field: %v", err)
			}
		}
		
		// Corrupt checksum (last 4 bytes if present)
		if len(corrupted) >= 8 {
			copy(corrupted[len(corrupted)-4:], []byte{0xFF, 0xFF, 0xFF, 0xFF})
			_, err := Decompress(nil, corrupted)
			// This might not fail if checksum isn't present
			_ = err
		}
	})
}

// FuzzDictionaryMismatch tests compression/decompression with wrong dictionaries
func FuzzDictionaryMismatch(f *testing.F) {
	f.Add([]byte("data"), []byte("dict1"), []byte("dict2"), 3)
	
	f.Fuzz(func(t *testing.T, data, dict1, dict2 []byte, level int) {
		if len(dict1) == 0 || len(dict2) == 0 || len(dict1) > 1<<20 || len(dict2) > 1<<20 {
			return
		}
		
		// Create two different dictionaries
		cd1, err := NewCDictLevel(dict1, level)
		if err != nil {
			return
		}
		defer cd1.Release()
		
		cd2, err := NewCDictLevel(dict2, level)
		if err != nil {
			return
		}
		defer cd2.Release()
		
		dd1, err := NewDDict(dict1)
		if err != nil {
			return
		}
		defer dd1.Release()
		
		dd2, err := NewDDict(dict2)
		if err != nil {
			return
		}
		defer dd2.Release()
		
		// Compress with dict1
		compressed := CompressDict(nil, data, cd1)
		
		// Try to decompress with dict2 (wrong dictionary)
		result, err := DecompressDict(nil, compressed, dd2)
		if err == nil {
			// This should usually fail unless dicts are very similar
			if bytes.Equal(result, data) {
				t.Error("Decompression with wrong dictionary produced correct result")
			} else {
				t.Logf("Decompression with wrong dictionary produced different result: %q vs %q",
					result, data)
			}
		} else {
			// Expected error
			t.Logf("Expected error with dictionary mismatch: %v", err)
		}
		
		// Try to decompress without any dictionary
		_, err = Decompress(nil, compressed)
		if err == nil {
			t.Error("Decompression without dictionary should have failed")
		}
		
		// Compress without dict, decompress with dict
		compressedNoDict := Compress(nil, data)
		result, err = DecompressDict(nil, compressedNoDict, dd1)
		if err != nil {
			t.Logf("Decompression with unnecessary dictionary failed: %v", err)
		} else if !bytes.Equal(result, data) {
			t.Error("Data corruption when using unnecessary dictionary")
		}
	})
}

// FuzzMixedStreams tests concatenated/mixed compressed streams
func FuzzMixedStreams(f *testing.F) {
	f.Add([]byte("first"), []byte("second"), []byte("third"), 3, 5, 9)
	
	f.Fuzz(func(t *testing.T, data1, data2, data3 []byte, 
		level1, level2, level3 int) {
		
		// Create different compressions
		comp1 := CompressLevel(nil, data1, level1)
		comp2 := CompressLevel(nil, data2, level2)
		comp3 := CompressLevel(nil, data3, level3)
		
		// Test concatenated streams
		concatenated := append(append(comp1, comp2...), comp3...)
		
		// Try to decompress concatenated data
		// ZSTD might only decompress the first frame
		result, err := Decompress(nil, concatenated)
		if err != nil {
			t.Logf("Decompression of concatenated streams failed: %v", err)
		} else {
			// Check what we got
			if bytes.Equal(result, data1) {
				t.Log("Only first frame was decompressed from concatenated streams")
			} else if bytes.Equal(result, append(append(data1, data2...), data3...)) {
				t.Log("All frames were decompressed from concatenated streams")
			} else {
				t.Error("Unexpected result from concatenated streams")
			}
		}
		
		// Test mixed partial streams
		if len(comp1) > 10 && len(comp2) > 10 {
			// Take first half of comp1 and second half of comp2
			mixed := append(comp1[:len(comp1)/2], comp2[len(comp2)/2:]...)
			
			_, err := Decompress(nil, mixed)
			if err == nil {
				t.Error("Decompression of mixed partial streams should have failed")
			}
		}
		
		// Test interleaved bytes
		maxLen := len(comp1)
		if len(comp2) < maxLen {
			maxLen = len(comp2)
		}
		
		interleaved := make([]byte, 0, maxLen*2)
		for i := 0; i < maxLen; i++ {
			interleaved = append(interleaved, comp1[i], comp2[i])
		}
		
		_, err = Decompress(nil, interleaved)
		if err == nil {
			t.Error("Decompression of interleaved streams should have failed")
		}
		
		// Test with garbage between valid frames
		garbage := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00}
		mixed := append(append(comp1, garbage...), comp2...)
		
		_, err = Decompress(nil, mixed)
		if err == nil {
			t.Log("Decompression succeeded despite garbage between frames")
		}
	})
}

// FuzzSizeFieldTampering tests modifying encoded sizes
func FuzzSizeFieldTampering(f *testing.F) {
	f.Add([]byte("test data for size tampering"), 100, 1000000)
	
	f.Fuzz(func(t *testing.T, data []byte, fakeSize1, fakeSize2 int) {
		// Create frame with content size
		ctx := NewCCtx()
		ctx.SetParameter(ZSTD_c_contentSizeFlag, 1)
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			return
		}
		
		// Try to find and modify size fields
		corrupted := make([]byte, len(compressed))
		copy(corrupted, compressed)
		
		// Look for potential size fields after frame header
		// This is a bit hacky but we're fuzzing...
		for i := 5; i < len(corrupted)-8; i++ {
			// Look for values that might be sizes
			val := binary.LittleEndian.Uint64(corrupted[i:i+8])
			if val == uint64(len(data)) {
				// Found content size - corrupt it
				binary.LittleEndian.PutUint64(corrupted[i:i+8], uint64(fakeSize1))
				
				_, err := Decompress(nil, corrupted)
				if err != nil {
					t.Logf("Expected error with tampered content size: %v", err)
				} else {
					t.Log("Decompression succeeded despite tampered content size")
				}
				
				// Restore and try different value
				binary.LittleEndian.PutUint64(corrupted[i:i+8], uint64(fakeSize2))
				_, err = Decompress(nil, corrupted)
				if err == nil && fakeSize2 != len(data) {
					t.Error("Decompression succeeded with wrong content size")
				}
				
				break
			}
		}
		
		// Test with pledged size mismatch
		ctx.Reset(ZSTD_reset_session_only)
		ctx.SetPledgedSrcSize(uint64(fakeSize1))
		
		// This might fail during compression
		compressed2, err := ctx.Compress(nil, data)
		if err != nil {
			t.Logf("Compression failed with wrong pledged size: %v", err)
		} else if len(compressed2) > 0 {
			// Try to decompress
			result, err := Decompress(nil, compressed2)
			if err != nil {
				t.Logf("Decompression of wrong-pledged data failed: %v", err)
			} else if !bytes.Equal(result, data) {
				t.Error("Data corruption with wrong pledged size")
			}
		}
	})
}
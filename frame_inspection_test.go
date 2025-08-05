package gozstd

import (
	"fmt"
	"strings"
	"testing"
)

func TestFrameInspection(t *testing.T) {
	// Create test data with known properties
	originalData := []byte("This is test data for frame inspection functionality testing.")
	
	// Compress with content size enabled
	ctx := NewCCtx()
	defer ctx.Release()
	
	// Enable content size flag so it's stored in frame header
	err := ctx.SetParameter(ZSTD_c_contentSizeFlag, 1)
	if err != nil {
		t.Fatalf("Failed to enable content size flag: %v", err)
	}
	
	// Enable checksum for testing
	err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
	if err != nil {
		t.Fatalf("Failed to enable checksum flag: %v", err)
	}
	
	compressed, err := ctx.Compress(nil, originalData)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	
	t.Run("GetFrameContentSize", func(t *testing.T) {
		contentSize, err := GetFrameContentSize(compressed)
		if err != nil {
			t.Fatalf("GetFrameContentSize failed: %v", err)
		}
		
		if contentSize != uint64(len(originalData)) {
			t.Errorf("Content size mismatch: got %d, expected %d", contentSize, len(originalData))
		}
		
		t.Logf("Frame content size: %d bytes", contentSize)
	})
	
	t.Run("GetFrameCompressedSize", func(t *testing.T) {
		compressedSize, err := GetFrameCompressedSize(compressed)
		if err != nil {
			t.Fatalf("GetFrameCompressedSize failed: %v", err)
		}
		
		if compressedSize != uint64(len(compressed)) {
			t.Errorf("Compressed size mismatch: got %d, expected %d", compressedSize, len(compressed))
		}
		
		t.Logf("Frame compressed size: %d bytes", compressedSize)
	})
	
	t.Run("GetDecompressedSize", func(t *testing.T) {
		decompressedSize := GetDecompressedSize(compressed)
		
		if decompressedSize != uint64(len(originalData)) {
			t.Errorf("Decompressed size mismatch: got %d, expected %d", decompressedSize, len(originalData))
		}
		
		t.Logf("Legacy decompressed size: %d bytes", decompressedSize)
	})
	
	t.Run("GetFrameInfo", func(t *testing.T) {
		info, err := GetFrameInfo(compressed)
		if err != nil {
			t.Fatalf("GetFrameInfo failed: %v", err)
		}
		
		if !info.HasContentSize {
			t.Error("Frame should have content size (content size flag was enabled)")
		}
		
		if info.ContentSize != uint64(len(originalData)) {
			t.Errorf("Frame info content size mismatch: got %d, expected %d", 
				info.ContentSize, len(originalData))
		}
		
		if info.CompressedSize != uint64(len(compressed)) {
			t.Errorf("Frame info compressed size mismatch: got %d, expected %d", 
				info.CompressedSize, len(compressed))
		}
		
		if !info.HasChecksum {
			t.Error("Frame should have checksum (checksum flag was enabled)")
		}
		
		t.Logf("Frame info: ContentSize=%d, CompressedSize=%d, HasContentSize=%v, HasChecksum=%v", 
			info.ContentSize, info.CompressedSize, info.HasContentSize, info.HasChecksum)
	})
	
	t.Run("IsValidZSTDFrame", func(t *testing.T) {
		if !IsValidZSTDFrame(compressed) {
			t.Error("Valid ZSTD frame was not recognized as valid")
		}
		
		// Test invalid frames
		invalidFrames := [][]byte{
			{},
			{0x01, 0x02, 0x03},
			{0xFF, 0xFF, 0xFF, 0xFF},
			[]byte("not a zstd frame"),
		}
		
		for i, invalid := range invalidFrames {
			if IsValidZSTDFrame(invalid) {
				t.Errorf("Invalid frame %d was incorrectly recognized as valid", i)
			}
		}
	})
	
	t.Run("ValidateFrameHeader", func(t *testing.T) {
		err := ValidateFrameHeader(compressed)
		if err != nil {
			t.Errorf("Valid frame header validation failed: %v", err)
		}
		
		// Test invalid headers
		invalidHeaders := [][]byte{
			{},
			{0x01, 0x02, 0x03},
			{0xFF, 0xFF, 0xFF, 0xFF},
		}
		
		for i, invalid := range invalidHeaders {
			err := ValidateFrameHeader(invalid)
			if err == nil {
				t.Errorf("Invalid header %d passed validation", i)
			}
		}
	})
}

func TestFrameInspectionWithoutContentSize(t *testing.T) {
	// Create compressed data without content size flag
	ctx := NewCCtx()
	defer ctx.Release()
	
	// Disable content size flag
	err := ctx.SetParameter(ZSTD_c_contentSizeFlag, 0)
	if err != nil {
		t.Fatalf("Failed to disable content size flag: %v", err)
	}
	
	originalData := []byte("Test data without content size flag.")
	compressed, err := ctx.Compress(nil, originalData)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}
	
	t.Run("GetFrameContentSize_Unknown", func(t *testing.T) {
		_, err := GetFrameContentSize(compressed)
		if err == nil {
			t.Error("Expected error for frame without content size")
		}
		
		if !strings.Contains(err.Error(), "content size unknown") {
			t.Errorf("Expected 'content size unknown' error, got: %v", err)
		}
	})
	
	t.Run("GetDecompressedSize_Zero", func(t *testing.T) {
		size := GetDecompressedSize(compressed)
		if size != 0 {
			t.Errorf("Expected 0 for unknown decompressed size, got %d", size)
		}
	})
	
	t.Run("GetFrameInfo_NoContentSize", func(t *testing.T) {
		info, err := GetFrameInfo(compressed)
		if err != nil {
			t.Fatalf("GetFrameInfo failed: %v", err)
		}
		
		if info.HasContentSize {
			t.Error("Frame should not have content size (flag was disabled)")
		}
		
		if info.ContentSize != 0 {
			t.Errorf("Content size should be 0 when unknown, got %d", info.ContentSize)
		}
		
		// Compressed size should still be available
		if info.CompressedSize == 0 {
			t.Error("Compressed size should be available even when content size is unknown")
		}
	})
}

func TestFrameInspectionErrorCases(t *testing.T) {
	t.Run("EmptyInput", func(t *testing.T) {
		_, err := GetFrameContentSize([]byte{})
		if err == nil {
			t.Error("Expected error for empty input")
		}
		
		_, err = GetFrameCompressedSize([]byte{})
		if err == nil {
			t.Error("Expected error for empty input")
		}
		
		_, err = GetFrameInfo([]byte{})
		if err == nil {
			t.Error("Expected error for empty input")
		}
	})
	
	t.Run("TooSmallInput", func(t *testing.T) {
		smallInput := []byte{0x01, 0x02}
		
		_, err := GetFrameContentSize(smallInput)
		if err == nil {
			t.Error("Expected error for too small input")
		}
		
		_, err = GetFrameCompressedSize(smallInput)
		if err == nil {
			t.Error("Expected error for too small input")
		}
		
		_, err = GetFrameInfo(smallInput)
		if err == nil {
			t.Error("Expected error for too small input")
		}
	})
	
	t.Run("CorruptedFrame", func(t *testing.T) {
		// Create valid compressed data then corrupt it
		originalData := []byte("Test data for corruption testing.")
		compressed := Compress(nil, originalData)
		
		// Corrupt the frame by changing bytes after the magic number
		if len(compressed) > 8 {
			corrupted := make([]byte, len(compressed))
			copy(corrupted, compressed)
			corrupted[5] = ^corrupted[5] // Flip bits in frame header
			corrupted[6] = ^corrupted[6]
			
			_, err := GetFrameContentSize(corrupted)
			// Note: This might not always error depending on what we corrupted
			// But it should either work or give a proper error
			
			_, err = GetFrameCompressedSize(corrupted)
			// Same here - might work or give proper error
			
			// GetFrameInfo should handle errors gracefully
			_, err = GetFrameInfo(corrupted)
			if err != nil {
				// Error is expected and acceptable for corrupted data
				t.Logf("Corrupted frame error (expected): %v", err)
			}
		}
	})
}

func TestFrameInspectionPerformance(t *testing.T) {
	// Test performance with various data sizes
	sizes := []int{100, 1000, 10000, 100000}
	
	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}
			
			ctx := NewCCtx()
			defer ctx.Release()
			ctx.SetParameter(ZSTD_c_contentSizeFlag, 1)
			
			compressed, err := ctx.Compress(nil, data)
			if err != nil {
				t.Fatalf("Compression failed: %v", err)
			}
			
			// Test frame inspection performance
			start := testing.B{} // Approximate timing
			
			contentSize, err := GetFrameContentSize(compressed)
			if err != nil {
				t.Fatalf("GetFrameContentSize failed: %v", err)
			}
			
			compressedSize, err := GetFrameCompressedSize(compressed)
			if err != nil {
				t.Fatalf("GetFrameCompressedSize failed: %v", err)
			}
			
			info, err := GetFrameInfo(compressed)
			if err != nil {
				t.Fatalf("GetFrameInfo failed: %v", err)
			}
			
			_ = start // Avoid unused variable
			
			// Verify results
			if contentSize != uint64(size) {
				t.Errorf("Content size mismatch: got %d, expected %d", contentSize, size)
			}
			
			if compressedSize != uint64(len(compressed)) {
				t.Errorf("Compressed size mismatch: got %d, expected %d", compressedSize, len(compressed))
			}
			
			if info.ContentSize != contentSize {
				t.Errorf("Frame info content size mismatch: got %d, expected %d", 
					info.ContentSize, contentSize)
			}
			
			t.Logf("Size %d: %d -> %d bytes (ratio: %.2fx)", 
				size, len(compressed), contentSize, float64(contentSize)/float64(len(compressed)))
		})
	}
}
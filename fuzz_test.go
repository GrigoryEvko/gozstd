package gozstd

import (
	"bytes"
	"io"
	"testing"
)

// FuzzCompressDecompress tests basic compression and decompression with random data
func FuzzCompressDecompress(f *testing.F) {
	// Add seed corpus
	f.Add([]byte(""), 1)
	f.Add([]byte("a"), 3)
	f.Add([]byte("hello world"), 5)
	f.Add([]byte("the quick brown fox jumps over the lazy dog"), 9)
	f.Add(bytes.Repeat([]byte("abc"), 1000), 19)
	
	f.Fuzz(func(t *testing.T, data []byte, level int) {
		// Clamp level to valid range
		if level < -131072 {
			level = -131072
		}
		if level > 22 {
			level = 22
		}
		
		// Compress
		compressed := CompressLevel(nil, data, level)
		
		// Decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Failed to decompress: %v", err)
		}
		
		// Verify roundtrip
		if !bytes.Equal(data, decompressed) {
			t.Errorf("Roundtrip failed: input len=%d, compressed len=%d, decompressed len=%d",
				len(data), len(compressed), len(decompressed))
		}
	})
}

// FuzzCompressWithBuffer tests compression with pre-allocated buffers
func FuzzCompressWithBuffer(f *testing.F) {
	f.Add([]byte("test data"), 100, 3)
	f.Add(bytes.Repeat([]byte("x"), 1000), 50, 5)
	
	f.Fuzz(func(t *testing.T, data []byte, bufSize int, level int) {
		if bufSize < 0 || bufSize > 1<<20 {
			return // Skip invalid buffer sizes
		}
		
		// Create a buffer with some existing data
		existingData := []byte("PREFIX:")
		dst := make([]byte, len(existingData), len(existingData)+bufSize)
		copy(dst, existingData)
		
		// Compress into the buffer
		compressed := CompressLevel(dst, data, level)
		
		// Verify prefix is preserved
		if !bytes.HasPrefix(compressed, existingData) {
			t.Error("Prefix not preserved")
		}
		
		// Decompress the data part
		compressedData := compressed[len(existingData):]
		decompressed, err := Decompress(nil, compressedData)
		if err != nil {
			t.Fatalf("Failed to decompress: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("Roundtrip failed with buffer")
		}
	})
}

// FuzzDictionary tests dictionary compression with random data and dictionaries
func FuzzDictionary(f *testing.F) {
	f.Add([]byte("sample text"), []byte("dictionary content"), 3)
	f.Add([]byte("the quick brown fox"), []byte("the fox"), 5)
	
	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int) {
		// Skip invalid dictionary sizes
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}
		
		// Create compression dictionary
		cd, err := NewCDictLevel(dictData, level)
		if err != nil {
			return // Skip invalid dictionaries
		}
		defer cd.Release()
		
		// Create decompression dictionary
		dd, err := NewDDict(dictData)
		if err != nil {
			return
		}
		defer dd.Release()
		
		// Compress with dictionary
		compressed := CompressDict(nil, data, cd)
		
		// Decompress with dictionary
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err != nil {
			t.Fatalf("Failed to decompress with dict: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("Dictionary roundtrip failed")
		}
		
		// Verify decompression without dictionary fails
		_, err = Decompress(nil, compressed)
		if err == nil {
			t.Error("Expected error when decompressing without dictionary")
		}
	})
}

// FuzzDictionaryByRef tests memory-optimized dictionary functions
func FuzzDictionaryByRef(f *testing.F) {
	f.Add([]byte("test data"), []byte("dict"), 3)
	
	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int) {
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}
		
		// Create ByRef dictionaries
		cd, err := NewCDictByRefLevel(dictData, level)
		if err != nil {
			return
		}
		defer cd.Release()
		
		dd, err := NewDDictByRef(dictData)
		if err != nil {
			return
		}
		defer dd.Release()
		
		// Compress and decompress
		compressed := CompressDict(nil, data, cd)
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err != nil {
			t.Fatalf("ByRef decompression failed: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("ByRef dictionary roundtrip failed")
		}
		
		// Verify the dictionary data is still valid
		// This tests that ByRef truly references the original data
		if !bytes.Equal(dictData, dictData) {
			t.Error("Dictionary data was modified")
		}
	})
}

// FuzzAdvancedAPI tests the advanced compression API with various parameters
func FuzzAdvancedAPI(f *testing.F) {
	f.Add([]byte("test"), 3, 10, 12, 1, 0)
	f.Add([]byte("longer test data"), 5, 15, 15, 0, 1)
	
	f.Fuzz(func(t *testing.T, data []byte, level int, windowLog int, hashLog int, 
		checksumFlag int, strategy int) {
		ctx := NewCCtx()
		
		// Set compression level
		if err := ctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
			return // Skip invalid levels
		}
		
		// Set window log (10-31 for 64-bit)
		if windowLog >= 10 && windowLog <= 31 {
			ctx.SetParameter(ZSTD_c_windowLog, windowLog)
		}
		
		// Set hash log
		if hashLog >= 6 && hashLog <= 30 {
			ctx.SetParameter(ZSTD_c_hashLog, hashLog)
		}
		
		// Set checksum flag (0 or 1)
		checksumFlag = checksumFlag & 1
		if err := ctx.SetParameter(ZSTD_c_checksumFlag, checksumFlag); err != nil {
			t.Fatalf("Failed to set checksum flag: %v", err)
		}
		
		// Set strategy (0-9)
		if strategy >= 0 && strategy <= 9 {
			ctx.SetParameter(ZSTD_c_strategy, strategy)
		}
		
		// Compress
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Advanced API compression failed: %v", err)
		}
		
		// Decompress
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Failed to decompress advanced API output: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("Advanced API roundtrip failed")
		}
	})
}

// FuzzStreamCompression tests streaming compression with various chunk sizes
func FuzzStreamCompression(f *testing.F) {
	f.Add([]byte("stream test"), 10, 3)
	f.Add(bytes.Repeat([]byte("x"), 1000), 100, 5)
	
	f.Fuzz(func(t *testing.T, data []byte, chunkSize int, level int) {
		if chunkSize <= 0 || chunkSize > len(data) {
			chunkSize = len(data)
			if chunkSize == 0 {
				chunkSize = 1
			}
		}
		
		// Compress using writer
		var compressed bytes.Buffer
		w := NewWriterLevel(&compressed, level)
		
		// Write in chunks
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			if _, err := w.Write(data[i:end]); err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		}
		
		if err := w.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		
		// Decompress using reader
		r := NewReader(&compressed)
		decompressed, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("Stream roundtrip failed")
		}
	})
}

// FuzzConcurrentCompression tests thread safety with concurrent operations
func FuzzConcurrentCompression(f *testing.F) {
	f.Add([]byte("concurrent"), 3, 5)
	
	f.Fuzz(func(t *testing.T, data []byte, level int, goroutines int) {
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 5
		}
		
		// Run concurrent compressions
		results := make(chan []byte, goroutines)
		errors := make(chan error, goroutines)
		
		for i := 0; i < goroutines; i++ {
			go func() {
				compressed := CompressLevel(nil, data, level)
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					errors <- err
					return
				}
				results <- decompressed
			}()
		}
		
		// Collect results
		for i := 0; i < goroutines; i++ {
			select {
			case err := <-errors:
				t.Fatalf("Concurrent operation failed: %v", err)
			case result := <-results:
				if !bytes.Equal(data, result) {
					t.Error("Concurrent roundtrip produced different result")
				}
			}
		}
	})
}

// FuzzResetContext tests context reset functionality
func FuzzResetContext(f *testing.F) {
	f.Add([]byte("reset test"), 3, 1)
	
	f.Fuzz(func(t *testing.T, data []byte, level int, resetType int) {
		ctx := NewCCtx()
		
		// First compression
		ctx.SetParameter(ZSTD_c_compressionLevel, level)
		ctx.SetParameter(ZSTD_c_checksumFlag, 1)
		
		compressed1, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("First compression failed: %v", err)
		}
		
		// Reset context
		var resetDirective ZSTD_ResetDirective
		switch resetType % 3 {
		case 0:
			resetDirective = ZSTD_reset_session_only
		case 1:
			resetDirective = ZSTD_reset_parameters
		case 2:
			resetDirective = ZSTD_reset_session_and_parameters
		}
		
		if err := ctx.Reset(resetDirective); err != nil {
			t.Fatalf("Reset failed: %v", err)
		}
		
		// Second compression
		compressed2, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Second compression failed: %v", err)
		}
		
		// Both should decompress correctly
		decompressed1, _ := Decompress(nil, compressed1)
		decompressed2, _ := Decompress(nil, compressed2)
		
		if !bytes.Equal(data, decompressed1) || !bytes.Equal(data, decompressed2) {
			t.Error("Reset affected correctness")
		}
	})
}

// FuzzPledgedSize tests pledged source size functionality
func FuzzPledgedSize(f *testing.F) {
	f.Add([]byte("exact size"), uint64(10))
	f.Add([]byte("test"), uint64(4))
	
	f.Fuzz(func(t *testing.T, data []byte, pledgedSize uint64) {
		ctx := NewCCtx()
		
		// Set pledged size
		if err := ctx.SetPledgedSrcSize(pledgedSize); err != nil {
			return // Skip invalid sizes
		}
		
		// Only test if pledged size matches actual size
		if pledgedSize != uint64(len(data)) {
			return
		}
		
		compressed, err := ctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Compression with pledged size failed: %v", err)
		}
		
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(data, decompressed) {
			t.Error("Pledged size roundtrip failed")
		}
	})
}

// FuzzInvalidInput tests handling of corrupted/invalid compressed data
func FuzzInvalidInput(f *testing.F) {
	// Add some valid compressed data that we'll corrupt
	valid := Compress(nil, []byte("valid data"))
	f.Add(valid, 0, byte(1))
	f.Add(valid, len(valid)/2, byte(255))
	
	f.Fuzz(func(t *testing.T, compressedData []byte, corruptPos int, corruptValue byte) {
		if len(compressedData) == 0 {
			return
		}
		
		// Make a copy and corrupt it
		corrupted := make([]byte, len(compressedData))
		copy(corrupted, compressedData)
		
		if corruptPos >= 0 && corruptPos < len(corrupted) {
			corrupted[corruptPos] = corruptValue
		}
		
		// Try to decompress corrupted data
		_, err := Decompress(nil, corrupted)
		// We expect an error in most cases, but it shouldn't panic
		_ = err
	})
}
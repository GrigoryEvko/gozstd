package gozstd

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// FuzzDictionaryByRefMemorySafety tests use-after-free scenarios with ByRef dictionaries
func FuzzDictionaryByRefMemorySafety(f *testing.F) {
	f.Add([]byte("test data"), []byte("dictionary"), 5, 100)

	f.Fuzz(func(t *testing.T, data []byte, dictData []byte, level int, modifyDelay int) {
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}

		// Create a copy that we'll modify later
		dictCopy := make([]byte, len(dictData))
		copy(dictCopy, dictData)

		// Create ByRef dictionaries
		cd, err := NewCDictByRefLevel(dictCopy, level)
		if err != nil {
			return
		}
		defer cd.Release()

		dd, err := NewDDictByRef(dictCopy)
		if err != nil {
			return
		}
		defer dd.Release()

		// Start compression
		compressed := CompressDict(nil, data, cd)

		// Modify the dictionary data after compression starts
		// This simulates use-after-free if the implementation doesn't hold the reference
		if modifyDelay > 0 && modifyDelay < 1000 {
			time.Sleep(time.Duration(modifyDelay) * time.Microsecond)
		}

		// Corrupt the dictionary
		for i := range dictCopy {
			dictCopy[i] = ^dictCopy[i]
		}

		// Force GC to try to reclaim memory
		runtime.GC()
		runtime.Gosched()

		// Try to decompress - this might crash if dictionary was freed
		decompressed, err := DecompressDict(nil, compressed, dd)
		if err == nil && len(decompressed) > 0 {
			// If it worked, the implementation might be holding a copy
			// or we got lucky with memory not being reused yet
			t.Logf("Decompression succeeded despite dictionary corruption")
		}

		// Try to trigger more issues by reallocating
		dictCopy = make([]byte, 1)
		_ = dictCopy // Mark as used to avoid ineffectual assignment warning
		runtime.GC()

		// Try another compression - this should definitely fail or crash
		// if the implementation is using freed memory
		_ = CompressDict(nil, data, cd)
	})
}

// FuzzCCtxPoolAbuse tests pool lifecycle bugs
func FuzzCCtxPoolAbuse(f *testing.F) {
	f.Add([]byte("test"), 3, 10)

	f.Fuzz(func(t *testing.T, data []byte, level int, goroutines int) {
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 10
		}

		// This tests the bug where NewCCtx() never returns contexts to pool
		var contexts []*CCtx

		// Create many contexts
		for i := 0; i < 100; i++ {
			ctx := NewCCtx()
			contexts = append(contexts, ctx)
		}

		// Use them concurrently
		var wg sync.WaitGroup
		errors := make(chan error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				// Use multiple contexts from different goroutines
				for j := 0; j < len(contexts); j++ {
					ctx := contexts[(idx+j)%len(contexts)]

					// This might race if contexts are pooled incorrectly
					if err := ctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
						errors <- err
						return
					}

					compressed, err := ctx.Compress(nil, data)
					if err != nil {
						errors <- err
						return
					}

					// Verify compression worked
					decompressed, err := Decompress(nil, compressed)
					if err != nil {
						errors <- err
						return
					}

					if string(decompressed) != string(data) {
						errors <- err
						return
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors - these are expected when contexts are misused
		errorCount := 0
		for err := range errors {
			errorCount++
			t.Logf("Expected pool abuse error #%d: %v", errorCount, err)
		}

		if errorCount == 0 {
			t.Logf("No errors occurred despite concurrent context abuse")
		} else {
			t.Logf("Pool abuse test completed with %d expected errors", errorCount)
		}

		// Note: We can't return contexts to pool because there's no API for it!
		// This is a design bug in the current implementation
	})
}

// FuzzDoubleFree tests multiple Release() calls
func FuzzDoubleFree(f *testing.F) {
	f.Add([]byte("dict"), 3, 2)

	f.Fuzz(func(t *testing.T, dictData []byte, level int, releaseCount int) {
		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}

		if releaseCount < 0 || releaseCount > 10 {
			releaseCount = 2
		}

		// Test CDict double free
		cd, err := NewCDictLevel(dictData, level)
		if err != nil {
			return
		}

		// Release multiple times - this might panic or corrupt memory
		for i := 0; i < releaseCount; i++ {
			cd.Release()
			// The pointer should be nil after first release
			// but let's see if it crashes
		}

		// Test DDict double free
		dd, err := NewDDict(dictData)
		if err != nil {
			return
		}

		for i := 0; i < releaseCount; i++ {
			dd.Release()
		}

		// Try to use after free
		defer func() {
			if r := recover(); r != nil {
				// Expected - using freed dictionary should panic
				t.Logf("Caught expected panic: %v", r)
			}
		}()

		// This should panic or cause undefined behavior
		_ = CompressDict(nil, []byte("test"), cd)
	})
}

// FuzzConcurrentPoolAccess tests race conditions in pool access
func FuzzConcurrentPoolAccess(f *testing.F) {
	f.Add([]byte("data"), 5, 20, 100)

	f.Fuzz(func(t *testing.T, data []byte, level int, goroutines int, iterations int) {
		if goroutines <= 0 || goroutines > 1000 {
			goroutines = 20
		}
		if iterations <= 0 || iterations > 10000 {
			iterations = 100
		}

		// Track statistics
		var compressions int64
		var errors int64

		var wg sync.WaitGroup
		start := make(chan struct{})

		// Launch goroutines
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Wait for start signal to maximize contention
				<-start

				for j := 0; j < iterations; j++ {
					// This internally uses pool.Get/Put
					compressed := CompressLevel(nil, data, level)

					// Add some work to increase chance of race
					sum := 0
					for _, b := range compressed {
						sum += int(b)
					}

					decompressed, err := Decompress(nil, compressed)
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					if string(decompressed) != string(data) {
						atomic.AddInt64(&errors, 1)
						continue
					}

					atomic.AddInt64(&compressions, 1)
				}
			}()
		}

		// Start all goroutines at once
		close(start)
		wg.Wait()

		if errors > 0 {
			t.Errorf("Pool race conditions caused %d errors out of %d attempts",
				errors, int64(goroutines*iterations))
		}

		t.Logf("Completed %d successful compressions", compressions)
	})
}

// FuzzUnsafePointerManipulation tests unsafe pointer usage
func FuzzUnsafePointerManipulation(f *testing.F) {
	f.Add([]byte("test"), 10, 0)
	f.Add([]byte("test"), 10, -1)
	f.Add([]byte("test"), 10, 1000000)

	f.Fuzz(func(t *testing.T, data []byte, size int, offset int) {
		if len(data) == 0 {
			return
		}

		// Create a slice with specific capacity
		if size < len(data) {
			size = len(data)
		}
		if size > 1<<20 {
			size = 1 << 20
		}

		buf := make([]byte, len(data), size)
		copy(buf, data)

		// Try to compress with modified slice header
		// This simulates what could happen with unsafe pointer manipulation
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Caught panic from unsafe manipulation: %v", r)
			}
		}()

		// Create a corrupted slice by modifying offset
		if offset != 0 && len(buf) > 1 {
			// This creates an invalid slice that might cause issues
			corruptedData := buf[1:]
			if offset > 0 && offset < len(corruptedData) {
				corruptedData = corruptedData[offset:]
			}

			// Try to compress corrupted slice
			_ = Compress(nil, corruptedData)
		}

		// Test with zero-length but non-nil slice
		emptySlice := buf[:0]
		compressed := Compress(nil, emptySlice)
		if len(compressed) > 0 {
			_, _ = Decompress(nil, compressed)
		}

		// Test with slice extending beyond original capacity
		// This is undefined behavior but let's see if it's caught
		header := (*struct {
			Data unsafe.Pointer
			Len  int
			Cap  int
		})(unsafe.Pointer(&buf))

		oldCap := header.Cap
		header.Cap = header.Cap * 2 // Lie about capacity

		// This might read beyond allocated memory
		defer func() {
			header.Cap = oldCap // Restore
		}()

		_ = CompressLevel(nil, buf, 3)
	})
}

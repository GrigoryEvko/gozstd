package gozstd

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// FuzzSharedCCtxRace tests multiple goroutines using same CCtx
func FuzzSharedCCtxRace(f *testing.F) {
	f.Add([]byte("test"), 10, 5, 100)

	f.Fuzz(func(t *testing.T, data []byte, goroutines, iterations, delayUs int) {
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 10
		}
		if iterations <= 0 || iterations > 1000 {
			iterations = 100
		}
		if delayUs < 0 || delayUs > 1000 {
			delayUs = 10
		}

		// Create a shared context - THIS IS UNSAFE!
		ctx := NewCCtx()

		var wg sync.WaitGroup
		var errors int64
		var successes int64

		// Race detector should catch this
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < iterations; j++ {
					// Concurrent parameter setting
					level := (id + j) % 20
					if err := ctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					// Add some delay to increase race window
					if delayUs > 0 {
						time.Sleep(time.Duration(delayUs) * time.Microsecond)
					}

					// Concurrent compression
					compressed, err := ctx.Compress(nil, data)
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					// More concurrent parameter changes
					ctx.SetParameter(ZSTD_c_checksumFlag, id&1)
					ctx.SetParameter(ZSTD_c_strategy, (id+j)%9)

					// Verify result
					decompressed, err := Decompress(nil, compressed)
					if err != nil || string(decompressed) != string(data) {
						atomic.AddInt64(&errors, 1)
					} else {
						atomic.AddInt64(&successes, 1)
					}

					// Random reset to cause more chaos
					if j%10 == 0 {
						ctx.Reset(ZSTD_reset_session_only)
					}
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Shared CCtx race: %d errors, %d successes out of %d attempts",
			errors, successes, int64(goroutines*iterations))

		// We expect errors or race detector to fire
		if errors == 0 && testing.Short() {
			t.Log("Warning: No errors detected in shared CCtx usage - might have race conditions")
		}
	})
}

// FuzzDictionaryReleaseRace tests releasing dictionary while in use
func FuzzDictionaryReleaseRace(f *testing.F) {
	f.Add([]byte("data"), []byte("dictionary"), 5, 10, 100)

	f.Fuzz(func(t *testing.T, data, dictData []byte,
		compressionThreads, releaseDelayMs, iterations int) {

		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}
		if compressionThreads <= 0 || compressionThreads > 20 {
			compressionThreads = 5
		}
		if releaseDelayMs < 0 || releaseDelayMs > 100 {
			releaseDelayMs = 10
		}
		if iterations <= 0 || iterations > 100 {
			iterations = 10
		}

		for iter := 0; iter < iterations; iter++ {
			// Create dictionary
			cd, err := NewCDict(dictData)
			if err != nil {
				continue
			}

			dd, err := NewDDict(dictData)
			if err != nil {
				cd.Release()
				continue
			}

			var wg sync.WaitGroup
			var compressErrors int64
			var decompressErrors int64
			done := make(chan struct{})

			// Start compression threads
			for i := 0; i < compressionThreads; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-done:
							return
						default:
							compressed := CompressDict(nil, data, cd)
							if len(compressed) == 0 {
								atomic.AddInt64(&compressErrors, 1)
								continue
							}

							result, err := DecompressDict(nil, compressed, dd)
							if err != nil || string(result) != string(data) {
								atomic.AddInt64(&decompressErrors, 1)
							}
						}
					}
				}()
			}

			// Release dictionary after delay - UNSAFE!
			time.Sleep(time.Duration(releaseDelayMs) * time.Millisecond)
			cd.Release()
			dd.Release()

			// Let threads run a bit more with freed dictionary
			time.Sleep(10 * time.Millisecond)
			close(done)
			wg.Wait()

			if compressErrors > 0 || decompressErrors > 0 {
				t.Logf("Dictionary release race: %d compress errors, %d decompress errors",
					compressErrors, decompressErrors)
			}
		}
	})
}

// FuzzPoolCorruption tests concurrent pool operations
func FuzzPoolCorruption(f *testing.F) {
	f.Add([]byte("test"), 20, 100, 5)

	f.Fuzz(func(t *testing.T, data []byte, goroutines, iterations, delayUs int) {
		if goroutines <= 0 || goroutines > 100 {
			goroutines = 20
		}
		if iterations <= 0 || iterations > 1000 {
			iterations = 100
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		var errors int64

		// Stress test the pool with rapid get/put
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				<-start // Synchronize start

				for j := 0; j < iterations; j++ {
					// Rapid compression/decompression to stress pool
					level := (id + j) % 20
					compressed := CompressLevel(nil, data, level)

					// Sometimes add delay
					if delayUs > 0 && j%10 == 0 {
						time.Sleep(time.Duration(delayUs) * time.Microsecond)
					}

					// Force GC occasionally to stress finalizers
					if j%50 == 0 {
						runtime.GC()
						runtime.Gosched()
					}

					decompressed, err := Decompress(nil, compressed)
					if err != nil || string(decompressed) != string(data) {
						atomic.AddInt64(&errors, 1)
					}

					// Also stress dictionary pools
					if j%5 == 0 {
						dict := []byte("small dict")
						cd, err := NewCDict(dict)
						if err == nil {
							_ = CompressDict(nil, data, cd)
							cd.Release()
						}
					}
				}
			}(i)
		}

		// Start all goroutines at once
		close(start)
		wg.Wait()

		if errors > 0 {
			t.Errorf("Pool corruption caused %d errors in %d operations",
				errors, int64(goroutines*iterations))
		}
	})
}

// FuzzParameterRacing tests concurrent parameter setting
func FuzzParameterRacing(f *testing.F) {
	f.Add([]byte("test"), uint32(0x12345678), 10)

	f.Fuzz(func(t *testing.T, data []byte, paramPattern uint32, goroutines int) {
		if goroutines <= 0 || goroutines > 50 {
			goroutines = 10
		}

		ctx := NewCCtx()

		var wg sync.WaitGroup
		start := make(chan struct{})
		var setErrors int64
		var compressErrors int64

		// Each goroutine sets different parameters
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-start

				// Set parameters based on ID and pattern
				params := []struct {
					param CParameter
					value int
				}{
					{ZSTD_c_compressionLevel, (int(paramPattern) + id) % 23},
					{ZSTD_c_windowLog, 10 + ((int(paramPattern>>8) + id) % 18)},
					{ZSTD_c_hashLog, 6 + ((int(paramPattern>>16) + id) % 24)},
					{ZSTD_c_strategy, (int(paramPattern>>24) + id) % 9},
					{ZSTD_c_checksumFlag, (id + int(paramPattern)) & 1},
				}

				for _, p := range params {
					err := ctx.SetParameter(p.param, p.value)
					if err != nil {
						atomic.AddInt64(&setErrors, 1)
					}
				}

				// Try compression with racing parameters
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					atomic.AddInt64(&compressErrors, 1)
				} else {
					// Verify result
					decompressed, err := Decompress(nil, compressed)
					if err != nil || string(decompressed) != string(data) {
						atomic.AddInt64(&compressErrors, 1)
					}
				}
			}(i)
		}

		close(start)
		wg.Wait()

		if setErrors > 0 || compressErrors > 0 {
			t.Logf("Parameter racing: %d set errors, %d compress errors",
				setErrors, compressErrors)
		}
	})
}

// FuzzByRefDictionaryRace tests racing with ByRef dictionary memory
func FuzzByRefDictionaryRace(f *testing.F) {
	f.Add([]byte("data"), []byte("dictionary"), 5, 10)

	f.Fuzz(func(t *testing.T, data []byte, dictData []byte,
		modifyThreads, compressThreads int) {

		if len(dictData) == 0 || len(dictData) > 1<<20 {
			return
		}
		if modifyThreads <= 0 || modifyThreads > 10 {
			modifyThreads = 5
		}
		if compressThreads <= 0 || compressThreads > 10 {
			compressThreads = 5
		}

		// Create mutable dictionary buffer
		dictBuffer := make([]byte, len(dictData))
		copy(dictBuffer, dictData)

		// Create ByRef dictionaries
		cd, err := NewCDictByRef(dictBuffer)
		if err != nil {
			return
		}
		defer cd.Release()

		dd, err := NewDDictByRef(dictBuffer)
		if err != nil {
			return
		}
		defer dd.Release()

		var wg sync.WaitGroup
		stop := make(chan struct{})
		var modifyCount int64
		var errors int64

		// Threads that modify the dictionary buffer
		for i := 0; i < modifyThreads; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						// Modify dictionary content - UNSAFE with ByRef!
						for j := range dictBuffer {
							dictBuffer[j] = byte((int(dictBuffer[j]) + id) % 256)
						}
						atomic.AddInt64(&modifyCount, 1)
						runtime.Gosched()
					}
				}
			}(i)
		}

		// Threads that use the dictionary
		for i := 0; i < compressThreads; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						compressed := CompressDict(nil, data, cd)
						if len(compressed) == 0 {
							atomic.AddInt64(&errors, 1)
							continue
						}

						// This might fail or produce wrong results
						// due to dictionary modification
						result, err := DecompressDict(nil, compressed, dd)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						} else if string(result) != string(data) {
							// Data corruption due to dictionary change
							atomic.AddInt64(&errors, 1)
						}
					}
				}
			}()
		}

		// Let it run briefly
		time.Sleep(50 * time.Millisecond)
		close(stop)
		wg.Wait()

		t.Logf("ByRef dictionary race: %d modifications, %d errors",
			modifyCount, errors)

		// Check results of dictionary modification during use
		if errors == 0 && modifyCount > 0 {
			t.Logf("ByRef dictionary handled %d modifications gracefully with no errors", modifyCount)
		} else if errors > 0 {
			t.Logf("ByRef dictionary modifications caused %d errors as expected", errors)
		}
	})
}

// FuzzConcurrentReset tests racing reset operations
func FuzzConcurrentReset(f *testing.F) {
	f.Add([]byte("test"), 5, 5, 100)

	f.Fuzz(func(t *testing.T, data []byte,
		resetThreads, compressThreads, iterations int) {

		if resetThreads <= 0 || resetThreads > 10 {
			resetThreads = 5
		}
		if compressThreads <= 0 || compressThreads > 10 {
			compressThreads = 5
		}
		if iterations <= 0 || iterations > 1000 {
			iterations = 100
		}

		ctx := NewCCtx()

		var wg sync.WaitGroup
		var resetCount int64
		var compressErrors int64

		// Threads that reset the context
		for i := 0; i < resetThreads; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < iterations; j++ {
					var directive ZSTD_ResetDirective = ZSTD_reset_session_only
					switch (id + j) % 3 {
					case 1:
						directive = ZSTD_reset_parameters
					case 2:
						directive = ZSTD_reset_session_and_parameters
					}

					err := ctx.Reset(directive)
					if err == nil {
						atomic.AddInt64(&resetCount, 1)
					}

					// Set some parameters after reset
					ctx.SetParameter(ZSTD_c_compressionLevel, (id+j)%20)
				}
			}(i)
		}

		// Threads that compress
		for i := 0; i < compressThreads; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for j := 0; j < iterations; j++ {
					compressed, err := ctx.Compress(nil, data)
					if err != nil {
						// Expected - reset might have cleared parameters
						atomic.AddInt64(&compressErrors, 1)
						// Try to fix by setting compression level
						ctx.SetParameter(ZSTD_c_compressionLevel, 3)
					} else {
						// Verify if we can
						result, err := Decompress(nil, compressed)
						if err != nil || string(result) != string(data) {
							atomic.AddInt64(&compressErrors, 1)
						}
					}
				}
			}()
		}

		wg.Wait()

		t.Logf("Concurrent reset: %d resets, %d compress errors out of %d attempts",
			resetCount, compressErrors, compressThreads*iterations)
	})
}

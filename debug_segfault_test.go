package gozstd

import (
	"fmt"
	"testing"
)

// TestSegfaultReproduction - Minimal test to reproduce the segmentation fault
func TestSegfaultReproduction(t *testing.T) {
	t.Skip("Rsync mode disabled due to ZSTD 1.5.7 segfault bug")
	// Create test data
	data := []byte("test data for segfault reproduction")

	t.Run("BasicCompress", func(t *testing.T) {
		// This should work fine
		cctx := NewCCtx()
		defer cctx.Release()

		_, err := cctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Basic compression failed: %v", err)
		}
		t.Log("Basic compression: OK")
	})

	t.Run("RsyncOnly", func(t *testing.T) {
		// Test rsync-friendly without multi-threading
		cctx := NewCCtx()
		defer cctx.Release()

		err := cctx.SetRsyncFriendly(true)
		if err != nil {
			t.Fatalf("SetRsyncFriendly failed: %v", err)
		}

		_, err = cctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Rsync-only compression failed: %v", err)
		}
		t.Log("Rsync-only compression: OK")
	})

	t.Run("MultiThreadingOnly", func(t *testing.T) {
		// Test multi-threading without rsync
		cctx := NewCCtx()
		defer cctx.Release()

		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		_, err = cctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Multi-threading only compression failed: %v", err)
		}
		t.Log("Multi-threading only compression: OK")
	})

	t.Run("RsyncWithMultiThreading", func(t *testing.T) {
		// This is the combination that should crash
		cctx := NewCCtx()
		defer cctx.Release()

		// Set parameters in the order from the benchmark
		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		err = cctx.SetRsyncFriendly(true)
		if err != nil {
			t.Fatalf("SetRsyncFriendly failed: %v", err)
		}

		err = cctx.SetParameter(ZSTD_c_compressionLevel, 5)
		if err != nil {
			t.Fatalf("SetParameter compressionLevel failed: %v", err)
		}

		// This is where the segfault should occur
		t.Log("About to call Compress with rsync + multi-threading...")
		_, err = cctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Rsync + multi-threading compression failed: %v", err)
		}
		t.Log("Rsync + multi-threading compression: OK")
	})
}

// TestBenchmarkReproduction - Test that matches the benchmark conditions exactly
func TestBenchmarkReproduction(t *testing.T) {
	t.Skip("Rsync mode disabled due to ZSTD 1.5.7 segfault bug")
	// Create data that matches the benchmark (10KB)
	data := newBenchString(10000)

	t.Run("RepeatedRsyncCompression", func(t *testing.T) {
		// Test repeated compression like the benchmark does
		cctx := NewCCtx()
		defer cctx.Release()

		// Set parameters exactly like the benchmark
		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		err = cctx.SetRsyncFriendly(true)
		if err != nil {
			t.Fatalf("SetRsyncFriendly failed: %v", err)
		}

		err = cctx.SetParameter(ZSTD_c_compressionLevel, 5)
		if err != nil {
			t.Fatalf("SetParameter compressionLevel failed: %v", err)
		}

		// Perform repeated compression like the benchmark
		for i := 0; i < 10; i++ {
			t.Logf("Compression attempt %d", i+1)
			compressed, err := cctx.Compress(nil, data)
			if err != nil {
				t.Fatalf("Compression %d failed: %v", i+1, err)
			}
			if len(compressed) == 0 {
				t.Fatalf("Compression %d returned empty result", i+1)
			}
		}
		t.Log("Repeated rsync compression: OK")
	})

	t.Run("ParallelRsyncCompression", func(t *testing.T) {
		// Test parallel compression using Go routines
		const numGoroutines = 4
		const numIterations = 5

		errChan := make(chan error, numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			go func(goroutineID int) {
				cctx := NewCCtx()
				defer cctx.Release()

				// Set parameters exactly like the benchmark
				err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d: SetParameter nbWorkers failed: %v", goroutineID, err)
					return
				}

				err = cctx.SetRsyncFriendly(true)
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d: SetRsyncFriendly failed: %v", goroutineID, err)
					return
				}

				err = cctx.SetParameter(ZSTD_c_compressionLevel, 5)
				if err != nil {
					errChan <- fmt.Errorf("goroutine %d: SetParameter compressionLevel failed: %v", goroutineID, err)
					return
				}

				// Perform compression iterations
				for i := 0; i < numIterations; i++ {
					compressed, err := cctx.Compress(nil, data)
					if err != nil {
						errChan <- fmt.Errorf("goroutine %d iteration %d: compression failed: %v", goroutineID, i+1, err)
						return
					}
					if len(compressed) == 0 {
						errChan <- fmt.Errorf("goroutine %d iteration %d: empty compression result", goroutineID, i+1)
						return
					}
				}

				errChan <- nil // Success
			}(g)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			if err := <-errChan; err != nil {
				t.Fatal(err)
			}
		}
		t.Log("Parallel rsync compression: OK")
	})
}

// TestCCtxStateValidation - Test for context state corruption
func TestCCtxStateValidation(t *testing.T) {
	t.Skip("Rsync mode disabled due to ZSTD 1.5.7 segfault bug")
	cctx := NewCCtx()
	defer cctx.Release()

	// Check if the underlying C context is valid
	if cctx.cctx == nil {
		t.Fatal("CCtx.cctx is NULL immediately after creation")
	}

	t.Logf("CCtx.cctx pointer: %p", cctx.cctx)

	// Set rsync parameters and check state
	err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
	if err != nil {
		t.Fatalf("SetParameter nbWorkers failed: %v", err)
	}

	if cctx.cctx == nil {
		t.Fatal("CCtx.cctx became NULL after SetParameter nbWorkers")
	}

	t.Logf("CCtx.cctx pointer after nbWorkers: %p", cctx.cctx)

	err = cctx.SetRsyncFriendly(true)
	if err != nil {
		t.Fatalf("SetRsyncFriendly failed: %v", err)
	}

	if cctx.cctx == nil {
		t.Fatal("CCtx.cctx became NULL after SetRsyncFriendly")
	}

	t.Logf("CCtx.cctx pointer after rsync: %p", cctx.cctx)
}

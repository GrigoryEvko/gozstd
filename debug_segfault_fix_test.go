package gozstd

import (
	"strings"
	"testing"
)

// TestSegfaultFix verifies that the rsync-friendly segfault has been fixed
func TestSegfaultFix(t *testing.T) {
	t.Run("RsyncableBlocked", func(t *testing.T) {
		// Test that rsync-friendly mode is now blocked for safety
		cctx := NewCCtx()
		defer cctx.Release()

		// Enable multi-threading first
		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		// Attempt to enable rsync-friendly mode - should be blocked
		err = cctx.SetRsyncFriendly(true)
		if err == nil {
			t.Fatal("Expected rsync-friendly mode to be blocked, but it succeeded")
		}

		// Verify the error message indicates it's blocked for safety
		errMsg := err.Error()
		if !strings.Contains(errMsg, "temporarily disabled") {
			t.Errorf("Expected error message to mention temporary disable, got: %v", err)
		}
		if !strings.Contains(errMsg, "segfault") {
			t.Errorf("Expected error message to mention segfault, got: %v", err)
		}
		if !strings.Contains(errMsg, "ZSTD 1.5.7") {
			t.Errorf("Expected error message to mention ZSTD version, got: %v", err)
		}

		t.Logf("Rsync-friendly mode correctly blocked with error: %v", err)
	})

	t.Run("DirectParameterBlocked", func(t *testing.T) {
		// Test that directly setting the rsync parameter is also blocked
		cctx := NewCCtx()
		defer cctx.Release()

		// Enable multi-threading first
		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		// Attempt to directly set rsync parameter - should be blocked
		err = cctx.SetParameter(ZSTD_c_rsyncable, 1)
		if err == nil {
			t.Fatal("Expected direct rsync parameter to be blocked, but it succeeded")
		}

		// Verify it's the safety error
		errMsg := err.Error()
		if !strings.Contains(errMsg, "temporarily disabled") {
			t.Errorf("Expected error message to mention temporary disable, got: %v", err)
		}

		t.Logf("Direct rsync parameter correctly blocked with error: %v", err)
	})

	t.Run("NormalCompressionStillWorks", func(t *testing.T) {
		// Test that normal compression (without rsync) still works perfectly
		cctx := NewCCtx()
		defer cctx.Release()

		// Enable multi-threading
		err := cctx.SetParameter(ZSTD_c_nbWorkers, 2)
		if err != nil {
			t.Fatalf("SetParameter nbWorkers failed: %v", err)
		}

		// Set compression level
		err = cctx.SetParameter(ZSTD_c_compressionLevel, 5)
		if err != nil {
			t.Fatalf("SetParameter compressionLevel failed: %v", err)
		}

		// Test data (same size as the benchmark that crashed)
		data := newBenchString(10000)

		// Perform multiple compressions to ensure stability
		for i := 0; i < 10; i++ {
			compressed, err := cctx.Compress(nil, data)
			if err != nil {
				t.Fatalf("Compression %d failed: %v", i+1, err)
			}
			if len(compressed) == 0 {
				t.Fatalf("Compression %d returned empty result", i+1)
			}

			// Verify we can decompress it back
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Decompression %d failed: %v", i+1, err)
			}
			if len(decompressed) != len(data) {
				t.Fatalf("Decompression %d size mismatch: got %d, want %d", i+1, len(decompressed), len(data))
			}
		}

		t.Log("Normal multi-threaded compression works correctly")
	})

	t.Run("SingleThreadedCompressionWorks", func(t *testing.T) {
		// Test that single-threaded compression works without issues
		cctx := NewCCtx()
		defer cctx.Release()

		// Set compression level (single-threaded by default)
		err := cctx.SetParameter(ZSTD_c_compressionLevel, 5)
		if err != nil {
			t.Fatalf("SetParameter compressionLevel failed: %v", err)
		}

		// Test data
		data := newBenchString(10000)

		// Single-threaded compression should work fine
		compressed, err := cctx.Compress(nil, data)
		if err != nil {
			t.Fatalf("Single-threaded compression failed: %v", err)
		}
		if len(compressed) == 0 {
			t.Fatal("Single-threaded compression returned empty result")
		}

		t.Log("Single-threaded compression works correctly")
	})
}

// TestSafetyMechanisms verifies that the safety mechanisms work as expected
func TestSafetyMechanisms(t *testing.T) {
	t.Run("ErrorContainsActionableInformation", func(t *testing.T) {
		cctx := NewCCtx()
		defer cctx.Release()

		// Enable multi-threading
		cctx.SetParameter(ZSTD_c_nbWorkers, 2)

		// Try to enable rsync mode
		err := cctx.SetRsyncFriendly(true)
		if err == nil {
			t.Fatal("Expected rsync mode to be blocked")
		}

		errMsg := err.Error()

		// Check that the error contains actionable information
		requiredPhrases := []string{
			"temporarily disabled",
			"segfault",
			"ZSTD 1.5.7",
			"safety measure",
			"prevent application crashes",
		}

		for _, phrase := range requiredPhrases {
			if !strings.Contains(errMsg, phrase) {
				t.Errorf("Error message missing required phrase '%s': %v", phrase, err)
			}
		}

		t.Logf("Error message contains all required information: %v", err)
	})

	t.Run("DisablingRsyncWorks", func(t *testing.T) {
		cctx := NewCCtx()
		defer cctx.Release()

		// Disabling rsync mode should always work
		err := cctx.SetRsyncFriendly(false)
		if err != nil {
			t.Errorf("Disabling rsync-friendly mode should always work, got error: %v", err)
		}

		// Setting rsync parameter to 0 should also work
		err = cctx.SetParameter(ZSTD_c_rsyncable, 0)
		if err != nil {
			t.Errorf("Setting rsync parameter to 0 should work, got error: %v", err)
		}

		t.Log("Disabling rsync mode works correctly")
	})
}

// TestFutureZstdUpgrade documents how to re-enable rsync mode when ZSTD is fixed
func TestFutureZstdUpgrade(t *testing.T) {
	t.Skip("This test documents the process for re-enabling rsync mode when ZSTD is upgraded")

	// When ZSTD is upgraded to a version that fixes the segfault:
	// 1. Update params_validation.go - uncomment the original validation logic
	// 2. Remove the safety check that returns the "temporarily disabled" error
	// 3. Update the ZSTD version detection in the validation
	// 4. Run comprehensive tests including the benchmark that originally crashed
	// 5. Update documentation to reflect that rsync mode is safe again
	
	t.Log("Instructions for re-enabling rsync mode:")
	t.Log("1. Upgrade ZSTD library to version > 1.5.7 with segfault fix")
	t.Log("2. Update validateRsyncableRequirements() in params_validation.go") 
	t.Log("3. Uncomment original validation logic")
	t.Log("4. Remove temporary safety block")
	t.Log("5. Run full test suite including BenchmarkAdvancedFeatures")
	t.Log("6. Update version detection and documentation")
}
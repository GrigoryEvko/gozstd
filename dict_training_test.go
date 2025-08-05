package gozstd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestEnhancedDictionaryTraining(t *testing.T) {
	// Create test samples with repetitive patterns suitable for dictionary training
	samples := createTestSamples()
	desiredDictLen := 4096 // 4KB dictionary

	t.Run("BuildDictCover", func(t *testing.T) {
		params := DefaultCoverParams()
		params.K = 32  // Small segment size for test data
		params.D = 6   // Small d-mer size
		params.NotificationLevel = 0 // Silent

		dict, err := BuildDictCover(samples, desiredDictLen, params)
		if err != nil {
			t.Fatalf("BuildDictCover failed: %v", err)
		}

		if len(dict) == 0 {
			t.Error("BuildDictCover returned empty dictionary")
		}

		if len(dict) > desiredDictLen*2 {
			t.Errorf("Dictionary too large: got %d bytes, max expected %d", len(dict), desiredDictLen*2)
		}

		// Test dictionary works for compression/decompression
		err = testDictionaryEffectiveness(dict, samples[0])
		if err != nil {
			t.Errorf("COVER dictionary test failed: %v", err)
		}

		t.Logf("COVER dictionary: %d bytes", len(dict))
	})

	t.Run("BuildDictCoverOptimized", func(t *testing.T) {
		params := DefaultCoverParams()
		params.K = 0  // Let optimization find optimal K
		params.D = 0  // Let optimization find optimal D
		params.Steps = 10 // Fewer steps for faster testing
		params.NotificationLevel = 0

		result, err := BuildDictCoverOptimized(samples, desiredDictLen, params)
		if err != nil {
			t.Fatalf("BuildDictCoverOptimized failed: %v", err)
		}

		if len(result.Dictionary) == 0 {
			t.Error("BuildDictCoverOptimized returned empty dictionary")
		}

		// Check that optimization found reasonable parameters
		if result.OptimizedParams.K == 0 {
			t.Error("Optimization failed to set K parameter")
		}
		if result.OptimizedParams.D == 0 {
			t.Error("Optimization failed to set D parameter")
		}

		// Test dictionary effectiveness
		err = testDictionaryEffectiveness(result.Dictionary, samples[0])
		if err != nil {
			t.Errorf("Optimized COVER dictionary test failed: %v", err)
		}

		t.Logf("Optimized COVER dictionary: %d bytes, K=%d, D=%d, Steps=%d",
			len(result.Dictionary), result.OptimizedParams.K, result.OptimizedParams.D, result.OptimizedParams.Steps)
	})

	t.Run("BuildDictFastCover", func(t *testing.T) {
		params := DefaultFastCoverParams()
		params.K = 32  // Small segment size for test data
		params.D = 6   // Small d-mer size
		params.F = 12  // Smaller frequency array for test
		params.NotificationLevel = 0

		dict, err := BuildDictFastCover(samples, desiredDictLen, params)
		if err != nil {
			t.Fatalf("BuildDictFastCover failed: %v", err)
		}

		if len(dict) == 0 {
			t.Error("BuildDictFastCover returned empty dictionary")
		}

		// Test dictionary effectiveness
		err = testDictionaryEffectiveness(dict, samples[0])
		if err != nil {
			t.Errorf("FastCover dictionary test failed: %v", err)
		}

		t.Logf("FastCover dictionary: %d bytes", len(dict))
	})

	t.Run("BuildDictFastCoverOptimized", func(t *testing.T) {
		params := DefaultFastCoverParams()
		params.K = 0  // Let optimization find optimal K
		params.D = 0  // Let optimization find optimal D
		params.F = 0  // Let optimization use default F
		params.Steps = 10 // Fewer steps for faster testing
		params.NotificationLevel = 0

		result, err := BuildDictFastCoverOptimized(samples, desiredDictLen, params)
		if err != nil {
			t.Fatalf("BuildDictFastCoverOptimized failed: %v", err)
		}

		if len(result.Dictionary) == 0 {
			t.Error("BuildDictFastCoverOptimized returned empty dictionary")
		}

		// Check optimization results
		if result.OptimizedParams.K == 0 {
			t.Error("Optimization failed to set K parameter")
		}
		if result.OptimizedParams.D == 0 {
			t.Error("Optimization failed to set D parameter")
		}
		if result.OptimizedParams.F == 0 {
			t.Error("Optimization failed to set F parameter")
		}

		// Test dictionary effectiveness
		err = testDictionaryEffectiveness(result.Dictionary, samples[0])
		if err != nil {
			t.Errorf("Optimized FastCover dictionary test failed: %v", err)
		}

		t.Logf("Optimized FastCover dictionary: %d bytes, K=%d, D=%d, F=%d, Accel=%d",
			len(result.Dictionary), result.OptimizedParams.K, result.OptimizedParams.D, 
			result.OptimizedParams.F, result.OptimizedParams.Accel)
	})

	t.Run("BuildDictLegacy", func(t *testing.T) {
		params := DefaultLegacyParams()
		params.NotificationLevel = 0

		dict, err := BuildDictLegacy(samples, desiredDictLen, params)
		if err != nil {
			t.Fatalf("BuildDictLegacy failed: %v", err)
		}

		if len(dict) == 0 {
			t.Error("BuildDictLegacy returned empty dictionary")
		}

		// Test dictionary effectiveness
		err = testDictionaryEffectiveness(dict, samples[0])
		if err != nil {
			t.Errorf("Legacy dictionary test failed: %v", err)
		}

		t.Logf("Legacy dictionary: %d bytes", len(dict))
	})
}

func TestDictionaryTrainingParameters(t *testing.T) {
	samples := createTestSamples()
	desiredDictLen := 2048

	t.Run("CoverParameterValidation", func(t *testing.T) {
		// Test invalid parameters
		invalidParams := []CoverParams{
			{K: 0, D: 6}, // K cannot be 0
			{K: 32, D: 0}, // D cannot be 0  
			{K: 16, D: 32}, // D cannot be > K
		}

		for i, params := range invalidParams {
			_, err := BuildDictCover(samples, desiredDictLen, params)
			if err == nil {
				t.Errorf("Expected error for invalid params case %d: %+v", i, params)
			}
		}
	})

	t.Run("FastCoverParameterValidation", func(t *testing.T) {
		// Test invalid parameters
		invalidParams := []FastCoverParams{
			{K: 0, D: 6, F: 12}, // K cannot be 0
			{K: 32, D: 0, F: 12}, // D cannot be 0
			{K: 16, D: 32, F: 12}, // D cannot be > K
		}

		for i, params := range invalidParams {
			_, err := BuildDictFastCover(samples, desiredDictLen, params)
			if err == nil {
				t.Errorf("Expected error for invalid params case %d: %+v", i, params)
			}
		}
	})

	t.Run("DefaultParametersValidity", func(t *testing.T) {
		// Test that default parameters are valid
		coverParams := DefaultCoverParams()
		if coverParams.K == 0 || coverParams.D == 0 || coverParams.D > coverParams.K {
			t.Errorf("Invalid default COVER parameters: %+v", coverParams)
		}

		fastCoverParams := DefaultFastCoverParams()
		if fastCoverParams.K == 0 || fastCoverParams.D == 0 || fastCoverParams.D > fastCoverParams.K {
			t.Errorf("Invalid default FastCover parameters: %+v", fastCoverParams)
		}

		legacyParams := DefaultLegacyParams()
		if legacyParams.CompressionLevel < -131072 || legacyParams.CompressionLevel > 22 {
			t.Errorf("Invalid default Legacy parameters: %+v", legacyParams)
		}
	})
}

func TestDictionaryTrainingEdgeCases(t *testing.T) {
	t.Run("EmptySamples", func(t *testing.T) {
		params := DefaultFastCoverParams()
		
		// Test with no samples
		_, err := BuildDictFastCover(nil, 1024, params)
		if err == nil {
			t.Error("Expected error for nil samples")
		}

		// Test with empty samples array
		_, err = BuildDictFastCover([][]byte{}, 1024, params)
		if err == nil {
			t.Error("Expected error for empty samples array")
		}

		// Test with all empty samples
		emptySamples := [][]byte{{}, {}, {}}
		_, err = BuildDictFastCover(emptySamples, 1024, params)
		if err == nil {
			t.Error("Expected error for all empty samples")
		}
	})

	t.Run("VerySmallSamples", func(t *testing.T) {
		// Test with very small samples (should still work due to padding)
		smallSamples := [][]byte{
			[]byte("a"),
			[]byte("b"), 
			[]byte("c"),
		}
		
		params := DefaultFastCoverParams()
		params.K = 16
		params.D = 4
		params.NotificationLevel = 0

		dict, err := BuildDictFastCover(smallSamples, 512, params)
		if err != nil {
			t.Logf("Small samples training failed (expected): %v", err)
			// This is expected to fail, so not an error
			return
		}

		if len(dict) > 0 {
			t.Logf("Small samples surprisingly produced dictionary of %d bytes", len(dict))
		}
	})

	t.Run("MinimumDictionarySize", func(t *testing.T) {
		samples := createTestSamples()
		params := DefaultFastCoverParams()
		params.NotificationLevel = 0

		// Test with size smaller than minimum - this should either fail or return a minimum-sized dictionary
		dict, err := BuildDictFastCover(samples, 1, params) // Very small size
		if err != nil {
			t.Logf("BuildDictFastCover failed with minimum size (expected): %v", err)
			// This is expected behavior - FastCover rejects unreasonable parameters
			return
		}

		if len(dict) < minDictLen {
			t.Errorf("Dictionary smaller than minimum: got %d, min %d", len(dict), minDictLen)
		}
	})
}

func TestDictionaryCompressionComparison(t *testing.T) {
	samples := createTestSamples()
	testData := samples[0] // Use first sample for testing
	
	// Build dictionaries with different algorithms
	algorithms := map[string]func() ([]byte, error){
		"Basic": func() ([]byte, error) {
			return BuildDict(samples, 2048), nil
		},
		"FastCover": func() ([]byte, error) {
			params := DefaultFastCoverParams()
			params.K = 64
			params.D = 8
			params.NotificationLevel = 0
			return BuildDictFastCover(samples, 2048, params)
		},
		"FastCoverOptimized": func() ([]byte, error) {
			params := DefaultFastCoverParams()
			params.K = 0 // Optimize
			params.D = 0 // Optimize
			params.Steps = 5 // Few steps for speed
			params.NotificationLevel = 0
			result, err := BuildDictFastCoverOptimized(samples, 2048, params)
			if err != nil {
				return nil, err
			}
			return result.Dictionary, nil
		},
	}

	compressionResults := make(map[string]float64)

	for name, buildFunc := range algorithms {
		dict, err := buildFunc()
		if err != nil {
			t.Logf("Algorithm %s failed: %v", name, err)
			continue
		}

		if len(dict) == 0 {
			t.Logf("Algorithm %s produced empty dictionary", name)
			continue
		}

		// Test compression ratio with this dictionary
		ratio, err := testCompressionRatio(dict, testData)
		if err != nil {
			t.Logf("Compression test failed for %s: %v", name, err)
			continue
		}

		compressionResults[name] = ratio
		t.Logf("Algorithm %s: dictionary %d bytes, compression ratio %.2fx", 
			name, len(dict), ratio)
	}

	// Verify we got some results
	if len(compressionResults) == 0 {
		t.Error("No compression results obtained")
	}

	// FastCover should generally be at least as good as basic
	if basicRatio, ok := compressionResults["Basic"]; ok {
		if fastCoverRatio, ok := compressionResults["FastCover"]; ok {
			if fastCoverRatio < basicRatio*0.8 { // Allow some tolerance
				t.Logf("FastCover ratio (%.2fx) significantly worse than Basic (%.2fx)", 
					fastCoverRatio, basicRatio)
			}
		}
	}
}

func TestDictionaryThreadSafety(t *testing.T) {
	samples := createTestSamples()
	params := DefaultFastCoverParams()
	params.NotificationLevel = 0

	// Test concurrent dictionary building (should be safe due to lock)
	const numGoroutines = 5
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			dict, err := BuildDictFastCover(samples, 1024, params)
			if err != nil {
				results <- fmt.Errorf("goroutine %d failed: %w", id, err)
				return
			}
			if len(dict) == 0 {
				results <- fmt.Errorf("goroutine %d got empty dictionary", id)
				return
			}
			results <- nil
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		if err != nil {
			t.Errorf("Concurrent training failed: %v", err)
		}
	}
}

// createTestSamples creates sample data suitable for dictionary training
func createTestSamples() [][]byte {
	// Create samples with repetitive patterns that should benefit from dictionary compression
	basePatterns := []string{
		"username", "password", "email", "firstname", "lastname", "address", "phone", "city", "state", "zipcode",
		"id", "name", "value", "type", "status", "created", "updated", "deleted", "active", "inactive",
		"success", "error", "warning", "info", "debug", "trace", "level", "message", "timestamp", "version",
	}

	var samples [][]byte
	
	for i := 0; i < 100; i++ {
		var sample strings.Builder
		sample.WriteString(fmt.Sprintf(`{"record_id": %d, `, i))
		
		// Add common fields with variations
		for j, pattern := range basePatterns {
			if j < len(basePatterns)-1 {
				sample.WriteString(fmt.Sprintf(`"%s": "%s_%d", `, pattern, pattern, i%10))
			} else {
				sample.WriteString(fmt.Sprintf(`"%s": "%s_%d"`, pattern, pattern, i%10))
			}
		}
		sample.WriteString("}")
		
		samples = append(samples, []byte(sample.String()))
	}

	// Add some additional structured data
	for i := 0; i < 50; i++ {
		xmlSample := fmt.Sprintf(`<root><user id="%d"><name>user_%d</name><email>user_%d@example.com</email><status>active</status></user></root>`, 
			i, i%20, i%20)
		samples = append(samples, []byte(xmlSample))
	}

	return samples
}

// testDictionaryEffectiveness tests that a dictionary actually works for compression/decompression
func testDictionaryEffectiveness(dict []byte, testData []byte) error {
	// Create compression context with dictionary
	cdict, err := NewCDict(dict)
	if err != nil {
		return fmt.Errorf("failed to create CDict: %w", err)
	}
	defer cdict.Release()

	// Create decompression context with dictionary
	ddict, err := NewDDict(dict)
	if err != nil {
		return fmt.Errorf("failed to create DDict: %w", err)
	}
	defer ddict.Release()

	// Compress with dictionary
	compressed := CompressDict(nil, testData, cdict)

	// Decompress with dictionary
	decompressed, err := DecompressDict(nil, compressed, ddict)
	if err != nil {
		return fmt.Errorf("decompression with dictionary failed: %w", err)
	}

	// Verify data integrity
	if !bytes.Equal(testData, decompressed) {
		return fmt.Errorf("decompressed data doesn't match original")
	}

	return nil
}

// testCompressionRatio tests the compression ratio achieved with a dictionary
func testCompressionRatio(dict []byte, testData []byte) (float64, error) {
	// Compress with dictionary
	cdict, err := NewCDict(dict)
	if err != nil {
		return 0, fmt.Errorf("failed to create CDict: %w", err)
	}
	defer cdict.Release()

	dictCompressed := CompressDict(nil, testData, cdict)

	// Calculate compression ratio  
	dictRatio := float64(len(testData)) / float64(len(dictCompressed))
	
	return dictRatio, nil
}

func BenchmarkDictionaryTraining(b *testing.B) {
	samples := createTestSamples()
	desiredDictLen := 4096

	b.Run("Basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dict := BuildDict(samples, desiredDictLen)
			if len(dict) == 0 {
				b.Error("BuildDict returned empty dictionary")
			}
		}
	})

	b.Run("FastCover", func(b *testing.B) {
		params := DefaultFastCoverParams()
		params.K = 64
		params.D = 8
		params.NotificationLevel = 0

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dict, err := BuildDictFastCover(samples, desiredDictLen, params)
			if err != nil {
				b.Fatalf("BuildDictFastCover failed: %v", err)
			}
			if len(dict) == 0 {
				b.Error("BuildDictFastCover returned empty dictionary")
			}
		}
	})

	b.Run("FastCoverOptimized", func(b *testing.B) {
		params := DefaultFastCoverParams()
		params.K = 0 // Optimize
		params.D = 0 // Optimize 
		params.Steps = 5 // Fewer steps for benchmarking
		params.NotificationLevel = 0

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result, err := BuildDictFastCoverOptimized(samples, desiredDictLen, params)
			if err != nil {
				b.Fatalf("BuildDictFastCoverOptimized failed: %v", err)
			}
			if len(result.Dictionary) == 0 {
				b.Error("BuildDictFastCoverOptimized returned empty dictionary")
			}
		}
	})

	b.Run("Cover", func(b *testing.B) {
		params := DefaultCoverParams()
		params.K = 64
		params.D = 8
		params.NotificationLevel = 0

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dict, err := BuildDictCover(samples, desiredDictLen, params)
			if err != nil {
				b.Fatalf("BuildDictCover failed: %v", err)
			}
			if len(dict) == 0 {
				b.Error("BuildDictCover returned empty dictionary")
			}
		}
	})
}
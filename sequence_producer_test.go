package gozstd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestSequenceStructureLayout(t *testing.T) {
	t.Run("StructureSizeValidation", func(t *testing.T) {
		// Test that ZSTD_Sequence Go struct matches C struct in size
		goSize := unsafe.Sizeof(ZSTD_Sequence{})
		expectedSize := uintptr(16) // 4 uint32 fields = 16 bytes
		
		if goSize != expectedSize {
			t.Errorf("ZSTD_Sequence size unexpected: got %d bytes, expected %d bytes", 
				goSize, expectedSize)
		}
		
		// Test field alignments
		seq := ZSTD_Sequence{}
		offsetOffset := unsafe.Offsetof(seq.Offset)
		litLengthOffset := unsafe.Offsetof(seq.LitLength)
		matchLengthOffset := unsafe.Offsetof(seq.MatchLength)
		repOffset := unsafe.Offsetof(seq.Rep)
		
		// Verify expected field offsets for packed struct
		if offsetOffset != 0 {
			t.Errorf("Offset field offset: got %d, expected 0", offsetOffset)
		}
		if litLengthOffset != 4 {
			t.Errorf("LitLength field offset: got %d, expected 4", litLengthOffset)
		}
		if matchLengthOffset != 8 {
			t.Errorf("MatchLength field offset: got %d, expected 8", matchLengthOffset)
		}
		if repOffset != 12 {
			t.Errorf("Rep field offset: got %d, expected 12", repOffset)
		}
		
		t.Logf("ZSTD_Sequence layout: size=%d, offsets=[%d,%d,%d,%d]", 
			goSize, offsetOffset, litLengthOffset, matchLengthOffset, repOffset)
	})
	
	t.Run("FieldValueAssignment", func(t *testing.T) {
		// Test that we can correctly assign values and they're stored properly
		seq := ZSTD_Sequence{
			Offset:      0x12345678,
			LitLength:   0x9ABCDEF0,
			MatchLength: 0x11223344,
			Rep:         0x55667788,
		}
		
		if seq.Offset != 0x12345678 {
			t.Errorf("Offset assignment failed: got 0x%x, expected 0x12345678", seq.Offset)
		}
		if seq.LitLength != 0x9ABCDEF0 {
			t.Errorf("LitLength assignment failed: got 0x%x, expected 0x9ABCDEF0", seq.LitLength)
		}
		if seq.MatchLength != 0x11223344 {
			t.Errorf("MatchLength assignment failed: got 0x%x, expected 0x11223344", seq.MatchLength)
		}
		if seq.Rep != 0x55667788 {
			t.Errorf("Rep assignment failed: got 0x%x, expected 0x55667788", seq.Rep)
		}
		
		t.Logf("Field value test passed with values: offset=0x%x, litLen=0x%x, matchLen=0x%x, rep=0x%x",
			seq.Offset, seq.LitLength, seq.MatchLength, seq.Rep)
	})
}

func TestSequenceProducerAPI(t *testing.T) {
	t.Run("SequenceBound", func(t *testing.T) {
		// Test sequence bound calculation
		testSizes := []int{0, 100, 1000, 10000, 100000}
		
		for _, size := range testSizes {
			bound := SequenceBound(size)
			if bound < 0 {
				t.Errorf("SequenceBound(%d) = %d, expected >= 0", size, bound)
			}
			
			// For non-zero sizes, bound should be reasonable relative to input size
			if size > 0 && bound > size*2 {
				t.Logf("Warning: SequenceBound(%d) = %d seems high", size, bound)
			}
			
			t.Logf("SequenceBound(%d) = %d", size, bound)
		}
	})
	
	t.Run("SequenceValidation", func(t *testing.T) {
		// Test valid sequences
		validSequences := []ZSTD_Sequence{
			{Offset: 0, LitLength: 10, MatchLength: 0, Rep: 0}, // All literals
		}
		
		err := ValidateSequences(validSequences, 10)
		if err != nil {
			t.Errorf("ValidateSequences failed for valid sequence: %v", err)
		}
		
		// Test invalid sequences - length mismatch
		invalidSequences := []ZSTD_Sequence{
			{Offset: 0, LitLength: 5, MatchLength: 0, Rep: 0}, // Only covers 5 bytes
		}
		
		err = ValidateSequences(invalidSequences, 10)
		if err == nil {
			t.Error("ValidateSequences should have failed for length mismatch")
		}
		
		// Test invalid final sequence
		invalidFinalSequence := []ZSTD_Sequence{
			{Offset: 4, LitLength: 0, MatchLength: 0, Rep: 0}, // Final sequence with offset != 0 and matchLength == 0
		}
		
		err = ValidateSequences(invalidFinalSequence, 0)
		if err == nil {
			t.Error("ValidateSequences should have failed for invalid final sequence")
		}
	})
	
	t.Run("MergeBlockDelimiters", func(t *testing.T) {
		// Test merging block delimiters
		sequences := []ZSTD_Sequence{
			{Offset: 0, LitLength: 5, MatchLength: 0, Rep: 0},  // Literals
			{Offset: 0, LitLength: 0, MatchLength: 0, Rep: 0},  // Block delimiter
			{Offset: 0, LitLength: 3, MatchLength: 0, Rep: 0},  // More literals
		}
		
		merged := MergeBlockDelimiters(sequences)
		
		// Should have fewer sequences after merging
		if len(merged) >= len(sequences) {
			t.Logf("Warning: MergeBlockDelimiters didn't reduce sequence count: %d -> %d", 
				len(sequences), len(merged))
		}
		
		t.Logf("MergeBlockDelimiters: %d -> %d sequences", len(sequences), len(merged))
	})
}

func TestSequenceProducerParameters(t *testing.T) {
	t.Run("SetSequenceValidation", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test enabling validation
		err := ctx.SetSequenceValidation(true)
		if err != nil {
			t.Fatalf("SetSequenceValidation(true) failed: %v", err)
		}
		
		// Test disabling validation
		err = ctx.SetSequenceValidation(false)
		if err != nil {
			t.Fatalf("SetSequenceValidation(false) failed: %v", err)
		}
	})
	
	t.Run("SetBlockDelimiters", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test setting block delimiter modes
		err := ctx.SetBlockDelimiters(ZSTD_sf_noBlockDelimiters)
		if err != nil {
			t.Fatalf("SetBlockDelimiters(noBlockDelimiters) failed: %v", err)
		}
		
		err = ctx.SetBlockDelimiters(ZSTD_sf_explicitBlockDelimiters)
		if err != nil {
			t.Fatalf("SetBlockDelimiters(explicitBlockDelimiters) failed: %v", err)
		}
	})
	
	t.Run("SetSequenceProducerFallback", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test enabling fallback
		err := ctx.SetSequenceProducerFallback(true)
		if err != nil {
			t.Fatalf("SetSequenceProducerFallback(true) failed: %v", err)
		}
		
		// Test disabling fallback
		err = ctx.SetSequenceProducerFallback(false)
		if err != nil {
			t.Fatalf("SetSequenceProducerFallback(false) failed: %v", err)
		}
	})
	
	t.Run("DirectParameterSetting", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Test setting parameters directly
		err := ctx.SetParameter(ZSTD_c_blockDelimiters, int(ZSTD_sf_noBlockDelimiters))
		if err != nil {
			t.Fatalf("SetParameter(blockDelimiters) failed: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_validateSequences, 1)
		if err != nil {
			t.Fatalf("SetParameter(validateSequences) failed: %v", err)
		}
		
		err = ctx.SetParameter(ZSTD_c_enableSeqProducerFallback, 1)
		if err != nil {
			t.Fatalf("SetParameter(enableSeqProducerFallback) failed: %v", err)
		}
	})
}

func TestSequenceProducerRegistration(t *testing.T) {
	t.Run("RegisterSimpleProducer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Register the simple sequence producer
		err := ctx.RegisterSequenceProducer(SimpleSequenceProducer)
		if err != nil {
			t.Fatalf("RegisterSequenceProducer failed: %v", err)
		}
		
		// Test compression with sequence producer
		testData := []byte("Hello, world! This is test data for sequence producer.")
		
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression with sequence producer failed: %v", err)
		}
		
		// Verify decompression
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(testData, decompressed) {
			t.Error("Sequence producer compression/decompression data mismatch")
		}
		
		t.Logf("Simple producer compression: %d -> %d bytes (ratio: %.2fx)", 
			len(testData), len(compressed), float64(len(testData))/float64(len(compressed)))
	})
	
	t.Run("RegisterRepetitiveProducer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Register the repetitive sequence producer
		err := ctx.RegisterSequenceProducer(RepetitiveSequenceProducer)
		if err != nil {
			t.Fatalf("RegisterSequenceProducer failed: %v", err)
		}
		
		// Create repetitive test data
		testData := bytes.Repeat([]byte("ABCD"), 100) // 400 bytes of "ABCDABCD..."
		
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression with repetitive producer failed: %v", err)
		}
		
		// Verify decompression
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(testData, decompressed) {
			t.Error("Repetitive producer compression/decompression data mismatch")
		}
		
		t.Logf("Repetitive producer compression: %d -> %d bytes (ratio: %.2fx)", 
			len(testData), len(compressed), float64(len(testData))/float64(len(compressed)))
	})
	
	t.Run("ClearSequenceProducer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Register a producer
		err := ctx.RegisterSequenceProducer(SimpleSequenceProducer)
		if err != nil {
			t.Fatalf("RegisterSequenceProducer failed: %v", err)
		}
		
		// Clear the producer
		err = ctx.ClearSequenceProducer()
		if err != nil {
			t.Fatalf("ClearSequenceProducer failed: %v", err)
		}
		
		// Test compression still works (using internal producer)
		testData := []byte("Test data after clearing sequence producer.")
		
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression after clearing producer failed: %v", err)
		}
		
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(testData, decompressed) {
			t.Error("Compression after clearing producer failed")
		}
		
		t.Logf("Post-clear compression: %d -> %d bytes", len(testData), len(compressed))
	})
}

func TestSequenceProducerErrors(t *testing.T) {
	t.Run("ErrorProducer", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Create a producer that always returns an error
		errorProducer := func(src []byte, dict []byte, compressionLevel int, windowSize uint64) ([]ZSTD_Sequence, error) {
			return nil, fmt.Errorf("test error")
		}
		
		// Enable fallback so compression doesn't fail
		err := ctx.SetSequenceProducerFallback(true)
		if err != nil {
			t.Fatalf("SetSequenceProducerFallback failed: %v", err)
		}
		
		err = ctx.RegisterSequenceProducer(errorProducer)
		if err != nil {
			t.Fatalf("RegisterSequenceProducer failed: %v", err)
		}
		
		testData := []byte("Test data for error producer with fallback.")
		
		// Should succeed due to fallback
		compressed, err := ctx.Compress(nil, testData)
		if err != nil {
			t.Fatalf("Compression with error producer and fallback failed: %v", err)
		}
		
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(testData, decompressed) {
			t.Error("Error producer with fallback failed")
		}
		
		t.Logf("Error producer with fallback: %d -> %d bytes", len(testData), len(compressed))
	})
	
	t.Run("InvalidSequences", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Create a producer that returns invalid sequences
		invalidProducer := func(src []byte, dict []byte, compressionLevel int, windowSize uint64) ([]ZSTD_Sequence, error) {
			// Return sequences that don't match the source length
			return []ZSTD_Sequence{
				{Offset: 0, LitLength: uint32(len(src) / 2), MatchLength: 0, Rep: 0},
			}, nil
		}
		
		// Enable sequence validation to catch the error
		err := ctx.SetSequenceValidation(true)
		if err != nil {
			t.Fatalf("SetSequenceValidation failed: %v", err)
		}
		
		// Enable fallback so we can test the validation
		err = ctx.SetSequenceProducerFallback(true)
		if err != nil {
			t.Fatalf("SetSequenceProducerFallback failed: %v", err)
		}
		
		err = ctx.RegisterSequenceProducer(invalidProducer)
		if err != nil {
			t.Fatalf("RegisterSequenceProducer failed: %v", err)
		}
		
		testData := []byte("Test data for invalid sequence producer.")
		
		// Should fail because sequence validation catches the error before fallback
		_, err = ctx.Compress(nil, testData)
		if err == nil {
			t.Error("Compression should have failed with invalid producer and validation enabled")
		}
		
		if !strings.Contains(err.Error(), "External sequences are not valid") {
			t.Errorf("Expected 'External sequences are not valid' error, got: %v", err)
		}
		
		t.Logf("Invalid producer correctly failed with validation: %v", err)
	})
}

func TestCompressSequences(t *testing.T) {
	t.Run("CompressLiteralSequence", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		testData := []byte("Hello, world!")
		
		// Create sequence that treats all data as literals
		sequences := []ZSTD_Sequence{
			{
				Offset:      0,
				LitLength:   uint32(len(testData)),
				MatchLength: 0,
				Rep:         0,
			},
		}
		
		// Validate the sequences
		err := ValidateSequences(sequences, len(testData))
		if err != nil {
			t.Fatalf("Sequence validation failed: %v", err)
		}
		
		// Compress using sequences
		compressed, err := ctx.CompressSequences(nil, sequences, testData)
		if err != nil {
			t.Fatalf("CompressSequences failed: %v", err)
		}
		
		// Verify decompression
		decompressed, err := Decompress(nil, compressed)
		if err != nil {
			t.Fatalf("Decompression failed: %v", err)
		}
		
		if !bytes.Equal(testData, decompressed) {
			t.Error("CompressSequences data mismatch")
		}
		
		t.Logf("CompressSequences: %d -> %d bytes", len(testData), len(compressed))
	})
	
	t.Run("CompressEmptySequences", func(t *testing.T) {
		ctx := NewCCtx()
		defer ctx.Release()
		
		testData := []byte("test")
		var sequences []ZSTD_Sequence
		
		// Should fail with empty sequences
		_, err := ctx.CompressSequences(nil, sequences, testData)
		if err == nil {
			t.Error("CompressSequences should have failed with empty sequences")
		}
		
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("Expected error about empty sequences, got: %v", err)
		}
	})
}

func TestSequenceProducerValidation(t *testing.T) {
	validator := NewParameterValidator()
	ctx := &ValidationContext{
		Architecture:  "amd64",
		CurrentParams: make(map[CParameter]int),
	}
	
	t.Run("BlockDelimitersValidation", func(t *testing.T) {
		// Valid values
		validValues := []int{0, 1} // ZSTD_sf_noBlockDelimiters, ZSTD_sf_explicitBlockDelimiters
		
		for _, value := range validValues {
			err := validator.ValidateParameter(ZSTD_c_blockDelimiters, value, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid blockDelimiters %d: %v", value, err)
			}
		}
		
		// Invalid values
		invalidValues := []int{-1, 2, 10}
		
		for _, value := range invalidValues {
			err := validator.ValidateParameter(ZSTD_c_blockDelimiters, value, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid blockDelimiters %d", value)
			}
		}
	})
	
	t.Run("ValidateSequencesValidation", func(t *testing.T) {
		// Valid values
		validValues := []int{0, 1}
		
		for _, value := range validValues {
			err := validator.ValidateParameter(ZSTD_c_validateSequences, value, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid validateSequences %d: %v", value, err)
			}
		}
		
		// Invalid values
		invalidValues := []int{-1, 2, 10}
		
		for _, value := range invalidValues {
			err := validator.ValidateParameter(ZSTD_c_validateSequences, value, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid validateSequences %d", value)
			}
		}
	})
	
	t.Run("EnableSeqProducerFallbackValidation", func(t *testing.T) {
		// Valid values
		validValues := []int{0, 1}
		
		for _, value := range validValues {
			err := validator.ValidateParameter(ZSTD_c_enableSeqProducerFallback, value, ctx)
			if err != nil {
				t.Errorf("Validation failed for valid enableSeqProducerFallback %d: %v", value, err)
			}
		}
		
		// Invalid values
		invalidValues := []int{-1, 2, 10}
		
		for _, value := range invalidValues {
			err := validator.ValidateParameter(ZSTD_c_enableSeqProducerFallback, value, ctx)
			if err == nil {
				t.Errorf("Validation should have failed for invalid enableSeqProducerFallback %d", value)
			}
		}
	})
}

func TestSequenceProducerIntegration(t *testing.T) {
	// Test complete integration of sequence producer API
	ctx := NewCCtx()
	defer ctx.Release()
	
	// Set up comprehensive sequence producer configuration
	err := ctx.SetParameter(ZSTD_c_compressionLevel, 5)
	if err != nil {
		t.Fatalf("SetParameter(compressionLevel) failed: %v", err)
	}
	
	err = ctx.SetBlockDelimiters(ZSTD_sf_noBlockDelimiters)
	if err != nil {
		t.Fatalf("SetBlockDelimiters failed: %v", err)
	}
	
	err = ctx.SetSequenceValidation(true)
	if err != nil {
		t.Fatalf("SetSequenceValidation failed: %v", err)
	}
	
	err = ctx.SetSequenceProducerFallback(true)
	if err != nil {
		t.Fatalf("SetSequenceProducerFallback failed: %v", err)
	}
	
	// Register custom sequence producer (use simple for reliability)
	customProducer := SimpleSequenceProducer
	
	err = ctx.RegisterSequenceProducer(customProducer)
	if err != nil {
		t.Fatalf("RegisterSequenceProducer failed: %v", err)
	}
	
	// Test with various data types
	testCases := [][]byte{
		[]byte("Simple short text."),
		[]byte("Longer text that should trigger the custom sequence producer logic for testing purposes."),
		bytes.Repeat([]byte("Pattern"), 20),
		make([]byte, 1000), // All zeros
	}
	
	for i, testData := range testCases {
		t.Run(fmt.Sprintf("Integration_Case_%d", i), func(t *testing.T) {
			compressed, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("Integration compression failed: %v", err)
			}
			
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("Integration decompression failed: %v", err)
			}
			
			if !bytes.Equal(testData, decompressed) {
				t.Error("Integration test data mismatch")
			}
			
			ratio := float64(len(testData)) / float64(len(compressed))
			t.Logf("Integration case %d: %d -> %d bytes (ratio: %.2fx)", 
				i, len(testData), len(compressed), ratio)
		})
	}
}
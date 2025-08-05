package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#include "zstd.h"
#include "zstd_errors.h"
*/
import "C"

import (
	"fmt"
)

// ZstdError represents a ZSTD-specific error with comprehensive information
type ZstdError struct {
	Code        int    // ZSTD error code
	Operation   string // What operation failed
	Message     string // Human-readable message from ZSTD
	Recoverable bool   // Whether the error can be recovered from
	Suggestion  string // Actionable suggestion for resolution
	Context     ErrorContext // Additional context information
}

// ErrorContext provides additional context for ZSTD errors
type ErrorContext struct {
	InputSize        int     // Size of input data
	OutputSize       int     // Size of output buffer  
	CompressionLevel int     // Compression level used
	DictionaryID     uint32  // Dictionary ID if applicable
	FrameInfo        string  // Frame information if available
}

// Error implements the error interface
func (e *ZstdError) Error() string {
	return fmt.Sprintf("ZSTD %s error: %s (code %d)", e.Operation, e.Message, e.Code)
}

// Specific error types for each category
type CorruptionError struct{ *ZstdError }      // Data corruption detected
type MemoryError struct{ *ZstdError }          // Memory allocation issues  
type BufferError struct{ *ZstdError }          // Buffer size issues
type ParameterError struct{ *ZstdError }       // Parameter validation
type DictionaryError struct{ *ZstdError }      // Dictionary problems
type StreamStateError struct{ *ZstdError }     // Stream state issues
type VersionError struct{ *ZstdError }         // Version compatibility
type FrameError struct{ *ZstdError }           // Frame format issues

// IsRecoverable returns whether the error can potentially be recovered from
func (e *ZstdError) IsRecoverable() bool {
	return e.Recoverable
}

// GetSuggestion returns an actionable suggestion for resolving the error
func (e *ZstdError) GetSuggestion() string {
	return e.Suggestion
}

// mapZstdError converts ZSTD error codes to comprehensive Go errors with context
func mapZstdError(result C.size_t, operation string, ctx ErrorContext) error {
	if !zstdIsError(result) {
		return nil
	}
	
	code := int(C.ZSTD_getErrorCode(result))
	message := C.GoString(C.ZSTD_getErrorString(C.ZSTD_getErrorCode(result)))
	
	baseError := &ZstdError{
		Code:      code,
		Operation: operation,
		Message:   message,
		Context:   ctx,
	}
	
	switch code {
	// Buffer and size errors
	case 70: // ZSTD_error_dstSize_tooSmall
		baseError.Recoverable = true
		baseError.Suggestion = fmt.Sprintf("Increase destination buffer size (current: %d bytes, try: %d bytes)", 
			ctx.OutputSize, ctx.OutputSize*2)
		return &BufferError{baseError}
		
	case 72: // ZSTD_error_srcSize_wrong
		baseError.Recoverable = false
		baseError.Suggestion = "Input size is invalid or corrupted"
		return &BufferError{baseError}
		
	case 74: // ZSTD_error_dstBuffer_null
		baseError.Recoverable = true
		baseError.Suggestion = "Provide a valid destination buffer"
		return &BufferError{baseError}
		
	case 104: // ZSTD_error_dstBuffer_wrong
		baseError.Recoverable = true
		baseError.Suggestion = "Destination buffer configuration is incorrect"
		return &BufferError{baseError}
		
	case 105: // ZSTD_error_srcBuffer_wrong
		baseError.Recoverable = false
		baseError.Suggestion = "Source buffer configuration is incorrect"
		return &BufferError{baseError}
	
	// Memory allocation errors
	case 64: // ZSTD_error_memory_allocation
		baseError.Recoverable = false
		baseError.Suggestion = fmt.Sprintf("Reduce compression level (current: %d) or free system memory", 
			ctx.CompressionLevel)
		return &MemoryError{baseError}
		
	case 66: // ZSTD_error_workSpace_tooSmall
		baseError.Recoverable = true
		baseError.Suggestion = "Increase workspace size for compression context"
		return &MemoryError{baseError}
	
	// Corruption and data integrity errors
	case 20: // ZSTD_error_corruption_detected
		baseError.Recoverable = false
		baseError.Suggestion = "Input data is corrupted and cannot be decompressed safely"
		return &CorruptionError{baseError}
		
	case 22: // ZSTD_error_checksum_wrong
		baseError.Recoverable = false
		baseError.Suggestion = "Data integrity check failed - possible corruption or tampering"
		return &CorruptionError{baseError}
		
	case 24: // ZSTD_error_literals_headerWrong
		baseError.Recoverable = false
		baseError.Suggestion = "Compressed data has invalid literals header"
		return &CorruptionError{baseError}
	
	// Dictionary errors
	case 30: // ZSTD_error_dictionary_corrupted
		baseError.Recoverable = false
		baseError.Suggestion = "Dictionary data is corrupted - obtain a valid dictionary"
		return &DictionaryError{baseError}
		
	case 32: // ZSTD_error_dictionary_wrong
		baseError.Recoverable = true
		if ctx.DictionaryID != 0 {
			baseError.Suggestion = fmt.Sprintf("Wrong dictionary (expected ID: %d) - use correct dictionary", ctx.DictionaryID)
		} else {
			baseError.Suggestion = "Frame requires a dictionary but none was provided"
		}
		return &DictionaryError{baseError}
		
	case 34: // ZSTD_error_dictionaryCreation_failed
		baseError.Recoverable = false
		baseError.Suggestion = "Dictionary creation failed - check training data quality"
		return &DictionaryError{baseError}
	
	// Parameter errors
	case 40: // ZSTD_error_parameter_unsupported
		baseError.Recoverable = true
		baseError.Suggestion = "Parameter not supported in this ZSTD version"
		return &ParameterError{baseError}
		
	case 41: // ZSTD_error_parameter_combination_unsupported
		baseError.Recoverable = true
		baseError.Suggestion = "Invalid parameter combination - check parameter constraints"
		return &ParameterError{baseError}
		
	case 42: // ZSTD_error_parameter_outOfBound
		baseError.Recoverable = true
		baseError.Suggestion = "Parameter value is outside valid range"
		return &ParameterError{baseError}
		
	case 44: // ZSTD_error_tableLog_tooLarge
		baseError.Recoverable = true
		baseError.Suggestion = "Table log parameter is too large - reduce value"
		return &ParameterError{baseError}
		
	case 46: // ZSTD_error_maxSymbolValue_tooLarge
		baseError.Recoverable = true
		baseError.Suggestion = "Maximum symbol value is too large"
		return &ParameterError{baseError}
		
	case 48: // ZSTD_error_maxSymbolValue_tooSmall
		baseError.Recoverable = true
		baseError.Suggestion = "Maximum symbol value is too small"
		return &ParameterError{baseError}
	
	// Frame and format errors
	case 10: // ZSTD_error_prefix_unknown
		baseError.Recoverable = false
		baseError.Suggestion = "Data does not start with valid ZSTD magic number"
		return &FrameError{baseError}
		
	case 14: // ZSTD_error_frameParameter_unsupported
		baseError.Recoverable = false
		baseError.Suggestion = "Frame uses unsupported parameters"
		return &FrameError{baseError}
		
	case 16: // ZSTD_error_frameParameter_windowTooLarge
		baseError.Recoverable = false
		baseError.Suggestion = "Frame window size exceeds maximum allowed"
		return &FrameError{baseError}
		
	case 100: // ZSTD_error_frameIndex_tooLarge
		baseError.Recoverable = false
		baseError.Suggestion = "Frame index is too large for seekable format"
		return &FrameError{baseError}
	
	// Stream state errors
	case 60: // ZSTD_error_stage_wrong
		baseError.Recoverable = true
		baseError.Suggestion = "Operation not valid in current stream stage"
		return &StreamStateError{baseError}
		
	case 62: // ZSTD_error_init_missing
		baseError.Recoverable = true
		baseError.Suggestion = "Stream context not properly initialized"
		return &StreamStateError{baseError}
		
	case 80: // ZSTD_error_noForwardProgress_destFull
		baseError.Recoverable = true
		baseError.Suggestion = "No progress possible - destination buffer is full"
		return &StreamStateError{baseError}
		
	case 82: // ZSTD_error_noForwardProgress_inputEmpty
		baseError.Recoverable = true
		baseError.Suggestion = "No progress possible - input buffer is empty"
		return &StreamStateError{baseError}
		
	case 102: // ZSTD_error_seekableIO
		baseError.Recoverable = false
		baseError.Suggestion = "Seekable IO operation failed"
		return &StreamStateError{baseError}
	
	// Version compatibility errors
	case 12: // ZSTD_error_version_unsupported
		baseError.Recoverable = false
		baseError.Suggestion = "Data was compressed with unsupported ZSTD version"
		return &VersionError{baseError}
	
	// Other specific errors
	case 49: // ZSTD_error_cannotProduce_uncompressedBlock
		baseError.Recoverable = true
		baseError.Suggestion = "Cannot produce uncompressed block - try different compression settings"
		return &ParameterError{baseError}
		
	case 50: // ZSTD_error_stabilityCondition_notRespected
		baseError.Recoverable = false
		baseError.Suggestion = "Internal stability condition violated - possible memory corruption"
		return &CorruptionError{baseError}
		
	case 106: // ZSTD_error_sequenceProducer_failed
		baseError.Recoverable = false
		baseError.Suggestion = "External sequence producer failed"
		return &StreamStateError{baseError}
		
	case 107: // ZSTD_error_externalSequences_invalid
		baseError.Recoverable = false
		baseError.Suggestion = "External sequences are invalid"
		return &StreamStateError{baseError}
	
	// Generic error (fallback)
	case 1: // ZSTD_error_GENERIC
		baseError.Recoverable = false
		baseError.Suggestion = "Generic ZSTD error - check input data and parameters"
		return baseError
		
	case 2: // Often returned for unspecified errors
		baseError.Recoverable = false
		baseError.Suggestion = "Unspecified ZSTD error - verify input data format and integrity"
		return &FrameError{baseError}
		
	default:
		baseError.Recoverable = false
		baseError.Suggestion = fmt.Sprintf("Unknown ZSTD error code %d - check ZSTD documentation", code)
		return baseError
	}
}

// Convenience functions for error type checking
func IsCorruptionError(err error) bool { _, ok := err.(*CorruptionError); return ok }
func IsMemoryError(err error) bool { _, ok := err.(*MemoryError); return ok }
func IsBufferError(err error) bool { _, ok := err.(*BufferError); return ok }
func IsParameterError(err error) bool { _, ok := err.(*ParameterError); return ok }
func IsDictionaryError(err error) bool { _, ok := err.(*DictionaryError); return ok }
func IsStreamStateError(err error) bool { _, ok := err.(*StreamStateError); return ok }
func IsVersionError(err error) bool { _, ok := err.(*VersionError); return ok }
func IsFrameError(err error) bool { _, ok := err.(*FrameError); return ok }

// Legacy errStr function for compatibility - will be phased out
func errStr(result C.size_t) string {
	errCode := C.ZSTD_getErrorCode(result)
	errCStr := C.ZSTD_getErrorString(errCode)
	return C.GoString(errCStr)
}
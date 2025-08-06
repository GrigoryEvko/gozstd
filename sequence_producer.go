// Package gozstd provides External Sequence Producer API support for ZSTD compression.
//
// OVERVIEW:
// The External Sequence Producer API allows custom compression algorithms to integrate
// with ZSTD at the block level. Instead of using ZSTD's internal sequence generation,
// you can provide your own function to analyze data and generate compression sequences.
//
// KEY FEATURES:
// - Block-level custom compression algorithms
// - Integration with existing ZSTD compression pipeline
// - Support for domain-specific compression logic
// - Fallback to internal algorithms on errors
// - Runtime sequence validation for safety
//
// IMPORTANT LIMITATIONS:
// - Experimental API requiring ZSTD_STATIC_LINKING_ONLY
// - Not compatible with Long Distance Matching (LDM)
// - Limited dictionary support
// - Thread safety requirements for sequence producers
//
// BASIC USAGE:
//
//	// Create compression context
//	ctx := NewCCtx()
//	defer ctx.Release()
//
//	// Register custom sequence producer
//	err := ctx.RegisterSequenceProducer(mySequenceProducer)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Enable safety features (recommended for development)
//	ctx.SetSequenceValidation(true)
//	ctx.SetSequenceProducerFallback(true)
//
//	// Compress data using custom producer
//	compressed, err := ctx.Compress(nil, data)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// PERFORMANCE CONSIDERATIONS:
// External sequence producers add overhead compared to ZSTD's optimized internal
// algorithms. Only use when you have domain-specific knowledge that can significantly
// improve compression ratio for your specific data types.
//
// See the SequenceProducer function type documentation for detailed requirements
// and examples of implementing custom sequence producers.
package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#include <stdint.h>
#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"

// C representation of ZSTD_Sequence for proper memory layout
typedef struct {
    unsigned int offset;
    unsigned int litLength;
    unsigned int matchLength;
    unsigned int rep;
} ZSTD_Sequence_C;

// Wrapper functions for sequence producer API
static size_t ZSTD_sequenceBound_wrapper(size_t srcSize) {
    return ZSTD_sequenceBound(srcSize);
}

static size_t ZSTD_compressSequences_wrapper(void *cctx, void *dst, size_t dstCapacity,
                                              void *inSeqs, size_t inSeqsSize,
                                              const void *src, size_t srcSize) {
    return ZSTD_compressSequences((ZSTD_CCtx*)cctx, dst, dstCapacity,
                                  (const ZSTD_Sequence*)inSeqs, inSeqsSize,
                                  src, srcSize);
}

static size_t ZSTD_mergeBlockDelimiters_wrapper(void *sequences, size_t seqsSize) {
    return ZSTD_mergeBlockDelimiters((ZSTD_Sequence*)sequences, seqsSize);
}

// Go sequence producer callback bridge
extern size_t goSequenceProducerCallback(size_t handle,
                                          void *outSeqs, size_t outSeqsCapacity,
                                          void *src, size_t srcSize,
                                          void *dict, size_t dictSize,
                                          int compressionLevel, size_t windowSize);

// C sequence producer that calls Go callback
static size_t sequenceProducerBridge(void *sequenceProducerState,
                                      ZSTD_Sequence *outSeqs, size_t outSeqsCapacity,
                                      const void *src, size_t srcSize,
                                      const void *dict, size_t dictSize,
                                      int compressionLevel, size_t windowSize) {
    size_t handle = (size_t)sequenceProducerState;
    return goSequenceProducerCallback(handle, outSeqs, outSeqsCapacity,
                                      (void*)src, srcSize, (void*)dict, dictSize,
                                      compressionLevel, windowSize);
}

static void ZSTD_registerSequenceProducer_wrapper(void *cctx, size_t handle) {
    if (handle == 0) {
        ZSTD_registerSequenceProducer((ZSTD_CCtx*)cctx, NULL, NULL);
    } else {
        ZSTD_registerSequenceProducer((ZSTD_CCtx*)cctx, (void*)handle, sequenceProducerBridge);
    }
}
*/
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// Compile-time verification that Go and C structures have the same memory layout
func init() {
	// Verify ZSTD_Sequence structure size matches between Go and C
	if unsafe.Sizeof(ZSTD_Sequence{}) != unsafe.Sizeof(C.ZSTD_Sequence_C{}) {
		panic(fmt.Sprintf("ZSTD_Sequence size mismatch: Go=%d bytes, C=%d bytes",
			unsafe.Sizeof(ZSTD_Sequence{}), unsafe.Sizeof(C.ZSTD_Sequence_C{})))
	}

	// Verify field alignment is correct
	goSeq := ZSTD_Sequence{}
	cSeq := C.ZSTD_Sequence_C{}

	if unsafe.Offsetof(goSeq.Offset) != uintptr(unsafe.Offsetof(cSeq.offset)) ||
		unsafe.Offsetof(goSeq.LitLength) != uintptr(unsafe.Offsetof(cSeq.litLength)) ||
		unsafe.Offsetof(goSeq.MatchLength) != uintptr(unsafe.Offsetof(cSeq.matchLength)) ||
		unsafe.Offsetof(goSeq.Rep) != uintptr(unsafe.Offsetof(cSeq.rep)) {
		panic("ZSTD_Sequence field alignment mismatch between Go and C")
	}
}

// ZSTD_Sequence represents a compression sequence (literal + match pair).
//
// A sequence describes a segment of the input data as either literals or a match:
// - Literals: Raw bytes that are stored as-is (offset=0, matchLength=0, litLength>0)
// - Matches: References to previous data at a given offset (offset>0, matchLength>0)
// - Block delimiters: Special sequences marking block boundaries (all fields=0)
//
// THREAD SAFETY: Sequence producers must be thread-safe as ZSTD may call them
// from multiple worker threads concurrently when multi-threading is enabled.
//
// PERFORMANCE: External sequence producers add overhead compared to ZSTD's internal
// algorithms. Use only when you have domain-specific knowledge that can improve
// compression ratio or when integrating with existing compression algorithms.
//
// VALIDATION: Enable ZSTD_c_validateSequences=1 to catch invalid sequences at runtime.
// This adds performance overhead but prevents corruption from malformed sequences.
type ZSTD_Sequence struct {
	// Offset: The offset of the match (NOT the same as the offset code)
	// - If offset == 0 and matchLength == 0: this sequence represents literals only
	// - If offset > 0: distance to the start of the match in the sliding window
	// - Must be <= windowSize when used as a match
	// - For block delimiters: offset == 0, matchLength == 0, litLength == 0
	Offset uint32

	// LitLength: Number of literal bytes that precede the match
	// - For literal-only sequences: total number of literal bytes
	// - For matches: number of literals before the match starts
	// - Total sequence length = litLength + matchLength
	// - Range: [0, UINT32_MAX] but practical limit depends on block size
	LitLength uint32

	// MatchLength: Length of the match in bytes
	// - 0 for literal-only sequences
	// - >= 3 for actual matches (ZSTD minimum match length)
	// - For final sequence in block: can be 0 even with offset != 0
	// - Range: [0, UINT32_MAX] but limited by remaining block data
	MatchLength uint32

	// Rep: Repeat offset indicator for advanced compression
	// - 0: 'offset' field contains the actual match offset
	// - 1-3: References to repeat offset history (advanced feature)
	// - Most external producers should set this to 0
	// - ZSTD will recalculate optimal repeat offsets during compression
	// - Range: [0, 3]
	Rep uint32
}

// SequenceProducerError is returned when a sequence producer encounters an error.
// This constant maps to ZSTD_SEQUENCE_PRODUCER_ERROR (size_t(-1)) in the C API.
const SequenceProducerError = ^uintptr(0)

// SequenceProducer is a function that generates compression sequences for a block of data.
//
// IMPORTANT REQUIREMENTS:
// 1. THREAD SAFETY: Must be safe to call concurrently from multiple goroutines
// 2. DETERMINISTIC: Should produce consistent output for the same inputs
// 3. COMPLETE COVERAGE: Sequences must account for ALL bytes in src
// 4. VALID SEQUENCES: Must follow ZSTD sequence format rules
//
// PARAMETERS:
// - src: Input data block to analyze and generate sequences for
// - dict: History buffer from previous blocks (currently always empty in ZSTD)
// - compressionLevel: ZSTD compression level hint for quality vs speed tradeoffs
// - windowSize: Maximum allowed match offset (sliding window size)
//
// RETURN VALUES:
// - []ZSTD_Sequence: Array of sequences that completely describe the input
// - error: Any error encountered during sequence generation
//
// SEQUENCE VALIDATION RULES:
// 1. Sum of all (litLength + matchLength) must equal len(src)
// 2. Match offsets must be <= windowSize and > 0 when matchLength > 0
// 3. Match lengths must be >= 3 (except for final sequence in block)
// 4. Sequences should be in order of appearance in the input
//
// PERFORMANCE CONSIDERATIONS:
// - External sequence producers add overhead vs ZSTD's internal algorithms
// - Simple literal-only producers may be slower than ZSTD's fast modes
// - Complex analysis can improve compression ratio for specific data types
// - Consider caching or pre-computed analysis for repeated patterns
//
// ERROR HANDLING:
// - Return error for invalid input or internal failures
// - ZSTD will fall back to internal producer if ZSTD_c_enableSeqProducerFallback=1
// - Returning empty sequences with non-empty src is considered an error
//
// EXAMPLE USAGE:
//
//	func MySequenceProducer(src []byte, dict []byte, level int, windowSize uint64) ([]ZSTD_Sequence, error) {
//	    if len(src) == 0 {
//	        return nil, fmt.Errorf("empty source data")
//	    }
//
//	    // Generate sequences that cover all bytes in src
//	    sequences := []ZSTD_Sequence{
//	        {Offset: 0, LitLength: uint32(len(src)), MatchLength: 0, Rep: 0},
//	    }
//
//	    return sequences, nil
//	}
type SequenceProducer func(
	src []byte, // Input data block to analyze
	dict []byte, // History buffer (currently always empty)
	compressionLevel int, // Compression level hint (1-22)
	windowSize uint64, // Maximum allowed match offset
) ([]ZSTD_Sequence, error)

// sequenceProducerRegistry manages registered sequence producers
type sequenceProducerRegistry struct {
	mutex     sync.RWMutex
	producers map[uintptr]SequenceProducer
	counter   uintptr
}

var globalSequenceProducerRegistry = &sequenceProducerRegistry{
	producers: make(map[uintptr]SequenceProducer),
	counter:   1,
}

// registerSequenceProducer registers a Go sequence producer and returns a handle
func (r *sequenceProducerRegistry) register(producer SequenceProducer) uintptr {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	handle := r.counter
	r.counter++
	r.producers[handle] = producer
	return handle
}

// unregister removes a sequence producer
func (r *sequenceProducerRegistry) unregister(handle uintptr) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.producers, handle)
}

// get retrieves a sequence producer by handle
func (r *sequenceProducerRegistry) get(handle uintptr) (SequenceProducer, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	producer, exists := r.producers[handle]
	return producer, exists
}

//export goSequenceProducerCallback
func goSequenceProducerCallback(handle C.size_t,
	outSeqs unsafe.Pointer, outSeqsCapacity C.size_t,
	src unsafe.Pointer, srcSize C.size_t,
	dict unsafe.Pointer, dictSize C.size_t,
	compressionLevel C.int, windowSize C.size_t) C.size_t {

	producer, exists := globalSequenceProducerRegistry.get(uintptr(handle))
	if !exists {
		return C.size_t(SequenceProducerError)
	}

	// Convert C parameters to Go
	var srcSlice []byte
	if srcSize > 0 {
		srcSlice = (*[1 << 30]byte)(src)[:srcSize:srcSize]
	}

	var dictSlice []byte
	if dictSize > 0 {
		dictSlice = (*[1 << 30]byte)(dict)[:dictSize:dictSize]
	}

	// Call the Go sequence producer
	sequences, err := producer(srcSlice, dictSlice, int(compressionLevel), uint64(windowSize))
	if err != nil {
		return C.size_t(SequenceProducerError)
	}

	// Check capacity
	if uintptr(len(sequences)) > uintptr(outSeqsCapacity) {
		return C.size_t(SequenceProducerError)
	}

	// Copy sequences to output buffer
	if len(sequences) > 0 {
		outSeqsSlice := (*[1 << 30]C.ZSTD_Sequence_C)(outSeqs)[:outSeqsCapacity:outSeqsCapacity]
		for i, seq := range sequences {
			outSeqsSlice[i].offset = C.uint(seq.Offset)
			outSeqsSlice[i].litLength = C.uint(seq.LitLength)
			outSeqsSlice[i].matchLength = C.uint(seq.MatchLength)
			outSeqsSlice[i].rep = C.uint(seq.Rep)
		}
	}

	return C.size_t(len(sequences))
}

// RegisterSequenceProducer registers an external sequence producer with the compression context.
//
// OVERVIEW:
// This enables block-level custom compression algorithms to integrate with ZSTD.
// The sequence producer will be called for each block during compression to generate
// sequences that describe how to encode the block data.
//
// IMPORTANT LIMITATIONS (from ZSTD documentation):
// 1. Long Distance Matching (LDM) is NOT supported - compression will fail
// 2. This is an EXPERIMENTAL API requiring ZSTD_STATIC_LINKING_ONLY
// 3. Not compatible with dictionary compression currently
//
// THREAD SAFETY:
// The sequence producer MUST be thread-safe as ZSTD may call it concurrently
// from multiple worker threads when nbWorkers > 0.
//
// PARAMETERS:
// - producer: Function to generate sequences, or nil to clear/disable
//
// RELATED SETTINGS:
// - ZSTD_c_validateSequences: Enable runtime sequence validation (recommended for development)
// - ZSTD_c_enableSeqProducerFallback: Fall back to internal producer on errors
// - ZSTD_c_blockDelimiters: Control block delimiter handling in sequences
//
// PERFORMANCE:
// External sequence producers add overhead. Only use when you have domain-specific
// knowledge that can significantly improve compression ratio.
//
// EXAMPLE:
//
//	err := ctx.RegisterSequenceProducer(myCustomProducer)
//	if err != nil {
//	    return fmt.Errorf("failed to register sequence producer: %w", err)
//	}
//	defer ctx.ClearSequenceProducer() // Clean up when done
//
// COMPATIBILITY:
// This setting persists until explicitly cleared or the compression context is reset.
// It's compatible with all ZSTD compression APIs that respect advanced parameters.
func (cctx *CCtx) RegisterSequenceProducer(producer SequenceProducer) error {
	if cctx == nil {
		return fmt.Errorf("compression context is nil")
	}

	if producer == nil {
		// Clear the sequence producer
		C.ZSTD_registerSequenceProducer_wrapper(unsafe.Pointer(cctx.cctx), 0)
		return nil
	}

	// Register the producer and get a handle
	handle := globalSequenceProducerRegistry.register(producer)

	// Register with ZSTD
	C.ZSTD_registerSequenceProducer_wrapper(unsafe.Pointer(cctx.cctx), C.size_t(handle))

	return nil
}

// ClearSequenceProducer removes any registered sequence producer.
//
// This reverts the compression context to using ZSTD's internal sequence generation
// algorithms. It's equivalent to calling RegisterSequenceProducer(nil).
//
// USAGE:
// Call this when you're done using external sequence production to ensure
// optimal performance for subsequent compressions.
func (cctx *CCtx) ClearSequenceProducer() error {
	return cctx.RegisterSequenceProducer(nil)
}

// SetSequenceValidation enables or disables runtime sequence validation.
//
// When enabled, ZSTD will validate that all sequences generated by external
// sequence producers follow the correct format and constraints:
// - Total sequence length matches source data length
// - Match offsets are within valid range
// - Match lengths meet minimum requirements
// - Final sequence constraints are respected
//
// PERFORMANCE IMPACT:
// Sequence validation adds overhead to compression but prevents corruption
// from malformed sequences. Recommended for development and testing.
//
// DEFAULT: Disabled (0) for maximum performance in production
//
// PARAMETERS:
// - enabled: true to enable validation, false to disable
func (cctx *CCtx) SetSequenceValidation(enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	return cctx.SetParameter(ZSTD_c_validateSequences, value)
}

// SetBlockDelimiters sets the block delimiter mode for sequence compression.
//
// This controls how ZSTD interprets sequences when using CompressSequences or
// external sequence producers:
//
// ZSTD_sf_noBlockDelimiters (default):
// - Sequences contain no explicit block boundaries
// - ZSTD determines block boundaries automatically based on block size
// - Sequences may be split across block boundaries
//
// ZSTD_sf_explicitBlockDelimiters:
// - Sequences contain explicit block delimiters (offset=0, matchLength=0, litLength=0)
// - ZSTD respects the specified block boundaries exactly
// - Enables advanced features like custom repcode resolution
//
// PARAMETERS:
// - format: ZSTD_sf_noBlockDelimiters or ZSTD_sf_explicitBlockDelimiters
func (cctx *CCtx) SetBlockDelimiters(format ZSTD_SequenceFormat) error {
	return cctx.SetParameter(ZSTD_c_blockDelimiters, int(format))
}

// SetSequenceProducerFallback enables or disables fallback to internal sequence producer.
//
// When enabled, ZSTD will automatically fall back to its internal sequence generation
// algorithms if the external sequence producer returns an error. This fallback
// happens on a block-by-block basis.
//
// FALLBACK BEHAVIOR:
// - Only affects blocks where the external producer returns an error
// - Fallback compression follows all other cParam settings (compression level, etc.)
// - Provides graceful degradation instead of compression failure
//
// PERFORMANCE:
// Enabling fallback adds minimal overhead but provides better reliability.
// Consider enabling in production environments for robustness.
//
// DEFAULT: Disabled (0) - compression fails if sequence producer fails
//
// PARAMETERS:
// - enabled: true to enable fallback, false to disable
func (cctx *CCtx) SetSequenceProducerFallback(enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	return cctx.SetParameter(ZSTD_c_enableSeqProducerFallback, value)
}

// SequenceBound returns the upper bound for the number of sequences that can be generated
// from a buffer of the given size.
//
// This function calculates the maximum possible number of sequences needed to represent
// a source buffer of the specified size. It's useful for pre-allocating sequence arrays
// in external sequence producers.
//
// CALCULATION:
// The bound is based on worst-case scenarios where every few bytes requires a separate
// sequence. The actual number of sequences needed is typically much smaller.
//
// USAGE:
//
//	bound := SequenceBound(len(sourceData))
//	sequences := make([]ZSTD_Sequence, 0, bound)
//	// Generate sequences up to 'bound' capacity
//
// PARAMETERS:
// - srcSize: Size of the source buffer in bytes
//
// RETURNS:
// - Maximum number of sequences that might be needed for the source size
func SequenceBound(srcSize int) int {
	return int(C.ZSTD_sequenceBound_wrapper(C.size_t(srcSize)))
}

// CompressSequences compresses an array of ZSTD_Sequence into the destination buffer.
//
// This function takes pre-generated sequences and uses them to compress the source
// data instead of using ZSTD's internal sequence generation algorithms.
//
// SEQUENCE REQUIREMENTS:
// 1. Sequences must represent a complete and valid parse of the source buffer
// 2. Sum of all (litLength + matchLength) must equal len(src)
// 3. Match offsets must be valid for the configured window size
// 4. Match lengths must meet ZSTD's minimum requirements (usually >= 3)
//
// COMPRESSION BEHAVIOR:
// The compression follows all cctx parameter settings:
// - Compression level affects entropy coding strength
// - Block delimiters setting affects sequence interpretation
// - Sequence validation setting affects error checking
//
// BUFFER MANAGEMENT:
// If dst doesn't have sufficient capacity, a new buffer will be allocated
// using the internal buffer pool system for efficient memory management.
//
// PARAMETERS:
// - dst: Destination buffer (will be extended if needed)
// - sequences: Array of sequences describing the compression
// - src: Source data to compress (must match sequence coverage)
//
// RETURNS:
// - Compressed data buffer (may be a new allocation)
// - Error if compression fails or sequences are invalid
//
// EXAMPLE:
//
//	sequences := []ZSTD_Sequence{
//	    {Offset: 0, LitLength: uint32(len(data)), MatchLength: 0, Rep: 0},
//	}
//	compressed, err := ctx.CompressSequences(nil, sequences, data)
func (cctx *CCtx) CompressSequences(dst []byte, sequences []ZSTD_Sequence, src []byte) ([]byte, error) {
	if cctx == nil {
		return nil, fmt.Errorf("compression context is nil")
	}

	if len(sequences) == 0 {
		return nil, fmt.Errorf("sequences array is empty")
	}

	dstLen := len(dst)

	// Calculate required capacity
	compressBound := int(C.ZSTD_compressBound(C.size_t(len(src)))) + 1
	requiredTotal := dstLen + compressBound

	if cap(dst) < requiredTotal {
		// Use buffer pool for more efficient memory management
		newBuf := GetBuffer(requiredTotal)
		copy(newBuf, dst[:dstLen])

		// Return old buffer to pool if it's from our pool system
		if cap(dst) > 0 && len(dst) > 0 {
			PutBuffer(dst[:0])
		}

		dst = newBuf[:dstLen]
	}

	// Prepare sequence array for C
	var seqPtr unsafe.Pointer
	if len(sequences) > 0 {
		seqPtr = unsafe.Pointer(&sequences[0])
	}

	// Prepare source data
	var srcPtr unsafe.Pointer
	if len(src) > 0 {
		srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))
		srcPtr = unsafe.Pointer(srcHdr.Data)
	}

	// Prepare destination buffer
	dstHdr := (*reflect.SliceHeader)(unsafe.Pointer(&dst))
	dstPtr := unsafe.Pointer(uintptr(dstHdr.Data) + uintptr(dstLen))

	// Call ZSTD
	result := C.ZSTD_compressSequences_wrapper(
		unsafe.Pointer(cctx.cctx),
		dstPtr,
		C.size_t(cap(dst)-dstLen),
		seqPtr,
		C.size_t(len(sequences)),
		srcPtr,
		C.size_t(len(src)))

	runtime.KeepAlive(dst)
	runtime.KeepAlive(src)
	runtime.KeepAlive(sequences)

	if C.ZSTD_isError(result) != 0 {
		ctx := ErrorContext{
			InputSize:  len(src),
			OutputSize: cap(dst) - dstLen,
		}
		return dst[:dstLen], mapZstdError(result, "sequence compression", ctx)
	}

	compressedSize := int(result)
	finalDst := dst[:dstLen+compressedSize]

	// Optimize buffer to reduce memory waste
	finalDst = OptimizeBuffer(finalDst)

	return finalDst, nil
}

// MergeBlockDelimiters removes block delimiters from a sequence array by merging them
// into the literals of the next sequence.
//
// Block delimiters are special sequences with offset=0, matchLength=0, and litLength=0
// that mark block boundaries. This function consolidates them by merging their literal
// content into subsequent sequences, producing a cleaner sequence array.
//
// USE CASES:
// - Converting from ZSTD_sf_explicitBlockDelimiters to ZSTD_sf_noBlockDelimiters format
// - Simplifying sequence arrays for analysis or debugging
// - Preparing sequences for contexts that don't support explicit delimiters
//
// ALGORITHM:
// The function identifies delimiter sequences and merges their effect into following
// sequences, reducing the total number of sequences while preserving semantics.
//
// PARAMETERS:
// - sequences: Input sequence array that may contain block delimiters
//
// RETURNS:
// - Modified sequence array with delimiters merged (slice of original backing array)
//
// NOTE:
// The returned slice uses the same backing array as the input but with reduced length.
// The original array may contain unmodified data beyond the returned slice.
func MergeBlockDelimiters(sequences []ZSTD_Sequence) []ZSTD_Sequence {
	if len(sequences) == 0 {
		return sequences
	}

	// Call ZSTD to merge delimiters
	seqPtr := unsafe.Pointer(&sequences[0])
	resultSize := C.ZSTD_mergeBlockDelimiters_wrapper(seqPtr, C.size_t(len(sequences)))

	runtime.KeepAlive(sequences)

	// Return the truncated slice
	return sequences[:resultSize]
}

// ValidateSequences checks if a sequence array represents a valid parse of the source data.
//
// This function performs comprehensive validation of sequence arrays to ensure they
// follow ZSTD's sequence format rules and completely cover the source data.
//
// VALIDATION CHECKS:
// 1. Complete coverage: Sum of all (litLength + matchLength) equals srcSize
// 2. Final sequence constraints: Last sequence with matchLength=0 must have offset=0
// 3. Match length requirements: Non-final matches must be >= 3 bytes (ZSTD_MINMATCH_MIN)
// 4. Non-empty source must have non-empty sequences
//
// ERROR CONDITIONS:
// - Empty sequences with non-empty source data
// - Sequence lengths don't sum to source size
// - Invalid final sequence format
// - Match lengths below ZSTD minimum requirements
//
// USAGE:
// Call this function to validate sequences before passing them to CompressSequences
// or when debugging external sequence producers:
//
//	if err := ValidateSequences(sequences, len(sourceData)); err != nil {
//	    return fmt.Errorf("invalid sequences: %w", err)
//	}
//
// PERFORMANCE:
// This validation is independent of ZSTD's internal validation (ZSTD_c_validateSequences).
// Use this for development/testing and enable ZSTD's validation for production safety.
//
// PARAMETERS:
// - sequences: Array of sequences to validate
// - srcSize: Expected total size of source data in bytes
//
// RETURNS:
// - nil if sequences are valid
// - Error describing the validation failure
func ValidateSequences(sequences []ZSTD_Sequence, srcSize int) error {
	if len(sequences) == 0 {
		if srcSize == 0 {
			return nil
		}
		return fmt.Errorf("no sequences provided for non-empty source")
	}

	totalLength := 0
	for i, seq := range sequences {
		totalLength += int(seq.LitLength) + int(seq.MatchLength)

		// Check final sequence constraints
		if i == len(sequences)-1 {
			if seq.MatchLength == 0 && seq.Offset != 0 {
				return fmt.Errorf("final sequence with matchLength=0 must have offset=0")
			}
		} else {
			// Non-final sequences must have sufficient match length
			if seq.MatchLength > 0 && seq.MatchLength < 3 { // ZSTD_MINMATCH_MIN
				return fmt.Errorf("sequence %d has matchLength %d < minimum (3)", i, seq.MatchLength)
			}
		}
	}

	if totalLength != srcSize {
		return fmt.Errorf("sequence lengths sum to %d but source size is %d", totalLength, srcSize)
	}

	return nil
}

// Example sequence producers
//
// The following functions demonstrate how to implement sequence producers for different
// use cases. They serve as both examples and ready-to-use producers for simple scenarios.

// SimpleSequenceProducer is a basic sequence producer that generates only literals.
//
// This producer treats all input data as literals (no matches), which is equivalent
// to uncompressed data from a sequence perspective. While this doesn't improve
// compression ratio, it demonstrates the minimum viable sequence producer implementation.
//
// USE CASES:
// - Testing and development
// - Baseline for comparing other sequence producers
// - Fallback when no matches can be found
// - Handling data types that don't benefit from match-based compression
//
// PERFORMANCE:
// This producer is very fast but produces larger output than ZSTD's internal algorithms
// since it doesn't identify any redundancy in the data.
//
// THREAD SAFETY: Yes - this function is stateless and thread-safe
//
// PARAMETERS:
// - src: Input data to process
// - dict: Dictionary data (unused by this producer)
// - compressionLevel: Compression level hint (unused by this producer)
// - windowSize: Window size limit (unused by this producer)
//
// RETURNS:
// - Single sequence covering all input data as literals
// - Error if input is empty
func SimpleSequenceProducer(src []byte, dict []byte, compressionLevel int, windowSize uint64) ([]ZSTD_Sequence, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("empty source data")
	}

	// Generate a single sequence that treats all data as literals
	sequences := []ZSTD_Sequence{
		{
			Offset:      0,
			LitLength:   uint32(len(src)),
			MatchLength: 0,
			Rep:         0,
		},
	}

	return sequences, nil
}

// RepetitiveSequenceProducer is a placeholder for pattern-based sequence generation.
//
// CURRENT IMPLEMENTATION:
// This function currently delegates to SimpleSequenceProducer for reliability.
// A full implementation would analyze the input data for repetitive patterns
// and generate match sequences to exploit redundancy.
//
// PLANNED FEATURES (for future implementation):
// - Detection of byte-level repetition patterns
// - Sliding window match finding
// - Optimal sequence selection based on compression benefit
// - Pattern length optimization
//
// WHY SIMPLIFIED:
// Implementing robust pattern detection requires:
// 1. Sophisticated pattern matching algorithms
// 2. Careful validation of generated sequences
// 3. Handling of edge cases and boundary conditions
// 4. Performance optimization for real-time compression
//
// THREAD SAFETY: Yes - delegates to thread-safe SimpleSequenceProducer
//
// EXTENSIBILITY:
// This function serves as a template for implementing custom pattern-based
// sequence producers. Replace the implementation with your domain-specific
// pattern detection logic while maintaining the same function signature.
//
// PARAMETERS:
// - src: Input data to analyze for patterns
// - dict: Dictionary data (reserved for future use)
// - compressionLevel: Compression level hint for quality/speed tradeoffs
// - windowSize: Maximum allowed match offset for sequence generation
//
// RETURNS:
// - Sequences generated by SimpleSequenceProducer (currently)
// - Error if sequence generation fails
func RepetitiveSequenceProducer(src []byte, dict []byte, compressionLevel int, windowSize uint64) ([]ZSTD_Sequence, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("empty source data")
	}

	// For now, just use the simple producer to ensure correctness
	// Real pattern detection is complex and error-prone
	return SimpleSequenceProducer(src, dict, compressionLevel, windowSize)
}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

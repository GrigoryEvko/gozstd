package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"
#include "zstd_errors.h"

// The following *_wrapper functions allow avoiding memory allocations
// during calls from Go.
// See https://github.com/golang/go/issues/24450 .

static size_t ZSTD_compressCCtx_wrapper(void *ctx, void *dst, size_t dstCapacity, const void *src, size_t srcSize, int compressionLevel) {
    return ZSTD_compressCCtx((ZSTD_CCtx*)ctx, dst, dstCapacity, src, srcSize, compressionLevel);
}

static size_t ZSTD_compress2_wrapper(void *ctx, void *dst, size_t dstCapacity, const void *src, size_t srcSize) {
    return ZSTD_compress2((ZSTD_CCtx*)ctx, dst, dstCapacity, src, srcSize);
}

static size_t ZSTD_compress_usingCDict_wrapper(void *ctx, void *dst, size_t dstCapacity, void *src, size_t srcSize, void *cdict) {
    return ZSTD_compress_usingCDict((ZSTD_CCtx*)ctx, (void*)dst, dstCapacity, (const void*)src, srcSize, (const ZSTD_CDict*)cdict);
}

static size_t ZSTD_decompressDCtx_wrapper(void *ctx, void *dst, size_t dstCapacity, void *src, size_t srcSize) {
    return ZSTD_decompressDCtx((ZSTD_DCtx*)ctx, (void*)dst, dstCapacity, (const void*)src, srcSize);
}

static size_t ZSTD_decompress_usingDDict_wrapper(void *ctx, void *dst, size_t dstCapacity, void *src, size_t srcSize, void *ddict) {
    return ZSTD_decompress_usingDDict((ZSTD_DCtx*)ctx, (void*)dst, dstCapacity, (const void*)src, srcSize, (const ZSTD_DDict*)ddict);
}

static unsigned long long ZSTD_findDecompressedSize_wrapper(void *src, size_t srcSize) {
    return ZSTD_findDecompressedSize((const void*)src, srcSize);
}

*/
import "C"

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// DefaultCompressionLevel is the default compression level.
const DefaultCompressionLevel = 3 // Obtained from ZSTD_CLEVEL_DEFAULT.

// Compress appends compressed src to dst and returns the result.
func Compress(dst, src []byte) []byte {
	return CompressDictLevel(dst, src, nil, DefaultCompressionLevel)
}

// CompressLevel appends compressed src to dst and returns the result.
//
// The given compressionLevel is used for the compression.
func CompressLevel(dst, src []byte, compressionLevel int) []byte {
	return CompressDictLevel(dst, src, nil, compressionLevel)
}

// CompressDict appends compressed src to dst and returns the result.
//
// The given dictionary is used for the compression.
func CompressDict(dst, src []byte, cd *CDict) []byte {
	return CompressDictLevel(dst, src, cd, 0)
}

func CompressDictLevel(dst, src []byte, cd *CDict, compressionLevel int) []byte {
	var cctx, cctxDict *cctxWrapper
	if cd == nil {
		cctx = cctxPool.Get().(*cctxWrapper)
	} else {
		cctxDict = cctxDictPool.Get().(*cctxWrapper)
	}

	dst = compress(cctx, cctxDict, dst, src, cd, compressionLevel)

	if cd == nil {
		cctxPool.Put(cctx)
	} else {
		cctxDictPool.Put(cctxDict)
	}
	return dst
}

var cctxPool = &sync.Pool{
	New: newCCtx,
}

var cctxDictPool = &sync.Pool{
	New: newCCtx,
}

func newCCtx() interface{} {
	cctx := C.ZSTD_createCCtx()
	cw := &cctxWrapper{
		cctx: cctx,
	}
	runtime.SetFinalizer(cw, freeCCtx)
	return cw
}

func freeCCtx(cw *cctxWrapper) {
	C.ZSTD_freeCCtx(cw.cctx)
	cw.cctx = nil
}

type cctxWrapper struct {
	cctx *C.ZSTD_CCtx
}

type CCtx struct {
	*cctxWrapper                     // Pointer to wrapper for proper pool lifecycle
	paramsMutex   sync.RWMutex       // Protect concurrent access to currentParams
	currentParams map[CParameter]int // Track parameters for validation
}

// NewCCtx creates a new compression context.
// The returned context must be released by calling Release() when no longer needed.
func NewCCtx() *CCtx {
	cctxWrap := cctxPool.Get().(*cctxWrapper)
	ctx := &CCtx{
		cctxWrapper:   cctxWrap, // Store pointer, not value
		currentParams: make(map[CParameter]int),
	}
	ctx.SetParameter(ZSTD_c_compressionLevel, 0)
	return ctx
}

// Release returns the compression context to the pool for reuse.
// The context must not be used after calling Release().
func (ctx *CCtx) Release() {
	if ctx == nil {
		return
	}
	// Reset the context to a clean state before returning to pool
	ctx.Reset(ZSTD_reset_session_and_parameters)

	// CRITICAL BUG FIX: Clear parameter tracking before returning to pool
	ctx.paramsMutex.Lock()
	ctx.currentParams = make(map[CParameter]int)
	ctx.paramsMutex.Unlock()

	// CRITICAL BUG FIX: Put the cctxWrapper pointer back in pool
	// Now that cctxWrapper is a pointer field, we can put it directly
	cctxPool.Put(ctx.cctxWrapper)
}

func (cctx *CCtx) Reset(reset ZSTD_ResetDirective) error {
	result := C.ZSTD_CCtx_reset(cctx.cctx,
		C.ZSTD_ResetDirective(reset))
	isErr := C.ZSTD_isError(C.size_t(result))
	if isErr != 0 {
		ctx := ErrorContext{}
		return mapZstdError(result, "reset context", ctx)
	}

	// Clear parameter tracking when context is reset
	if reset == ZSTD_reset_session_and_parameters || reset == ZSTD_reset_parameters {
		cctx.paramsMutex.Lock()
		cctx.currentParams = make(map[CParameter]int)
		cctx.paramsMutex.Unlock()
	}

	return nil
}

// Global parameter validator instance for memory bomb prevention
var globalValidator = NewParameterValidator()

// SetParameter sets compression parameters for the given context with comprehensive validation
func (cctx *CCtx) SetParameter(param CParameter, value int) error {
	// CRITICAL FIX: Hold lock for entire operation to prevent race conditions
	cctx.paramsMutex.Lock()
	defer cctx.paramsMutex.Unlock()

	// Initialize currentParams if needed
	if cctx.currentParams == nil {
		cctx.currentParams = make(map[CParameter]int)
	}

	// Create validation context with current parameters
	currentParamsCopy := make(map[CParameter]int, len(cctx.currentParams))
	for k, v := range cctx.currentParams {
		currentParamsCopy[k] = v
	}

	// CRITICAL: Validate parameter to prevent memory bomb attacks
	ctx := &ValidationContext{
		Architecture:  runtime.GOARCH,
		CurrentParams: currentParamsCopy,
	}

	// Perform comprehensive validation BEFORE calling ZSTD
	if err := globalValidator.ValidateParameter(param, value, ctx); err != nil {
		return err
	}

	// Add new parameter to validation set
	currentParamsCopy[param] = value

	// Validate parameter dependencies with new parameter included
	if err := ValidateParameterDependencies(currentParamsCopy); err != nil {
		return err
	}

	// Call ZSTD with validated parameters
	result := C.ZSTD_CCtx_setParameter(cctx.cctx,
		C.ZSTD_cParameter(param), C.int(value))
	isErr := C.ZSTD_isError(C.size_t(result))
	if isErr != 0 {
		errorCtx := ErrorContext{
			CompressionLevel: value,
		}
		return mapZstdError(result, "set parameter", errorCtx)
	}

	// Only update tracking after successful ZSTD call
	cctx.currentParams[param] = value

	return nil
}

// SetTargetBlockSize sets the target compressed block size.
// A value of 0 means auto-detection (default).
// Non-zero values attempt to fit compressed blocks around the target size.
// This is useful for network streaming and storage optimization.
func (cctx *CCtx) SetTargetBlockSize(targetSize int) error {
	return cctx.SetParameter(ZSTD_c_targetCBlockSize, targetSize)
}

// SetRsyncFriendly enables or disables rsync-friendly compression mode.
// When enabled (1), creates periodic synchronization points in the compressed stream
// that make it more suitable for rsync delta transfers and incremental backups.
// This slightly reduces compression ratio but improves rsync efficiency.
func (cctx *CCtx) SetRsyncFriendly(enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	return cctx.SetParameter(ZSTD_c_rsyncable, value)
}

// SetSourceSizeHint provides a hint about the expected input size.
// When the hint is close to the actual size, it can improve compression ratio.
// Set to 0 to disable the hint (default behavior).
func (cctx *CCtx) SetSourceSizeHint(sizeHint uint64) error {
	return cctx.SetParameter(ZSTD_c_srcSizeHint, int(sizeHint))
}

// SetStableInputBuffer enables or disables stable input buffer mode.
// When enabled, tells the compressor that input data will ALWAYS be the same
// between calls, avoiding memory copies but requiring strict buffer management.
// Use with caution - violating the contract will cause compression failures.
func (cctx *CCtx) SetStableInputBuffer(enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	return cctx.SetParameter(ZSTD_c_stableInBuffer, value)
}

// SetStableOutputBuffer enables or disables stable output buffer mode.
// When enabled, tells the compressor that output buffer will not be resized
// between calls, allowing direct compression without intermediate copies.
// Use with caution - buffer must be large enough for the entire operation.
func (cctx *CCtx) SetStableOutputBuffer(enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	return cctx.SetParameter(ZSTD_c_stableOutBuffer, value)
}

/*
*  Total input data size to be compressed as a single frame.
*  Value will be written in frame header, unless if explicitly forbidden using ZSTD_c_contentSizeFlag.
*  This value will also be controlled at end of frame, and trigger an error if not respected.
* @result : 0, or an error code (which can be tested with ZSTD_isError()).
*  Note 1 : pledgedSrcSize==0 actually means zero, aka an empty frame.
*           In order to mean "unknown content size", pass constant ZSTD_CONTENTSIZE_UNKNOWN.
*           ZSTD_CONTENTSIZE_UNKNOWN is default value for any new frame.
*  Note 2 : pledgedSrcSize is only valid once, for the next frame.
*           It's discarded at the end of the frame, and replaced by ZSTD_CONTENTSIZE_UNKNOWN.
*  Note 3 : Whenever all input data is provided and consumed in a single round,
*           for example with ZSTD_compress2(),
*           or invoking immediately ZSTD_compressStream2(,,,ZSTD_e_end),
*           this value is automatically overridden by srcSize instead.
 */
func (cctx *CCtx) SetPledgedSrcSize(PledgedSrcSize uint64) error {
	result := C.ZSTD_CCtx_setPledgedSrcSize(cctx.cctx,
		C.ulonglong(PledgedSrcSize))
	isErr := C.ZSTD_isError(C.size_t(result))
	if isErr != 0 {
		ctx := ErrorContext{
			InputSize: int(PledgedSrcSize),
		}
		return mapZstdError(result, "set pledged source size", ctx)
	}
	return nil
}

func (cctx *CCtx) Compress(dst, src []byte) ([]byte, error) {
	return compress2(cctx.cctxWrapper, dst, src)
}

func compress(cctx, cctxDict *cctxWrapper, dst, src []byte, cd *CDict, compressionLevel int) []byte {
	// ZSTD handles empty input correctly by creating valid frames, so don't skip it

	dstLen := len(dst)
	if cap(dst) > dstLen {
		// Fast path - try compressing without dst resize.
		result := compressInternal(cctx, cctxDict, dst[dstLen:cap(dst)], src, cd, compressionLevel, false)
		compressedSize := int(result)
		if compressedSize >= 0 {
			// All OK.
			return dst[:dstLen+compressedSize]
		}
		if C.ZSTD_getErrorCode(result) != C.ZSTD_error_dstSize_tooSmall {
			// Unexpected error - return as error instead of panicking
			ctx := ErrorContext{
				InputSize:  len(src),
				OutputSize: cap(dst) - dstLen,
			}
			if cd != nil {
				ctx.CompressionLevel = cd.compressionLevel
			}
			// This is a serious error - log and return it
			err := mapZstdError(result, "compression", ctx)
			panic(fmt.Errorf("BUG: unexpected error during compression with cd=%p: %w", cd, err))
		}
	}

	// Slow path - resize dst to fit compressed data using buffer pool
	compressBound := int(C.ZSTD_compressBound(C.size_t(len(src)))) + 1
	requiredTotal := dstLen + compressBound

	if cap(dst) < requiredTotal {
		// Use buffer pool for more efficient memory management
		newBuf := GetBuffer(requiredTotal)
		
		if dstLen > 0 {
			// Only extend to what we need to copy
			newBuf = newBuf[:dstLen]
			copy(newBuf, dst[:dstLen])
		}

		// Return old buffer to pool if it's from our pool system
		if cap(dst) > 0 && len(dst) > 0 {
			PutBuffer(dst[:0])
		}

		dst = newBuf[:dstLen]
	}

	result := compressInternal(cctx, cctxDict, dst[dstLen:dstLen+compressBound], src, cd, compressionLevel, true)
	compressedSize := int(result)
	dst = dst[:dstLen+compressedSize]

	// Optimize buffer to reduce memory waste
	dst = OptimizeBuffer(dst)

	return dst
}

func compress2(cctx *cctxWrapper, dst, src []byte) ([]byte, error) {
	// ZSTD handles empty input correctly by creating valid frames, so don't skip it

	dstLen := len(dst)

	// Build error context
	ctx := ErrorContext{
		InputSize:        len(src),
		OutputSize:       cap(dst) - dstLen,
		CompressionLevel: 0, // compress2 uses default level
	}

	if cap(dst) > dstLen {
		// Fast path - try compressing without dst resize.
		result := compress2Internal(cctx, dst[dstLen:cap(dst)], src, false)
		compressedSize := int(result)
		if compressedSize >= 0 {
			// All OK.
			return dst[:dstLen+compressedSize], nil
		}

		if C.ZSTD_getErrorCode(result) != C.ZSTD_error_dstSize_tooSmall {
			// Return proper error with context
			return dst, mapZstdError(result, "compression", ctx)
		}
	}

	// Slow path - resize dst to fit compressed data using buffer pool
	compressBound := int(C.ZSTD_compressBound(C.size_t(len(src)))) + 1
	requiredTotal := dstLen + compressBound

	if cap(dst) < requiredTotal {
		// Use buffer pool for more efficient memory management
		newBuf := GetBuffer(requiredTotal)
		
		if dstLen > 0 {
			// Only extend to what we need to copy
			newBuf = newBuf[:dstLen]
			copy(newBuf, dst[:dstLen])
		}

		// Return old buffer to pool if it's from our pool system
		if cap(dst) > 0 && len(dst) > 0 {
			PutBuffer(dst[:0])
		}

		dst = newBuf[:dstLen]
	}

	result := compress2Internal(cctx, dst[dstLen:dstLen+compressBound], src, false)
	compressedSize := int(result)
	if int(result) >= 0 {
		finalDst := dst[:dstLen+compressedSize]
		// Optimize buffer to reduce memory waste
		finalDst = OptimizeBuffer(finalDst)
		return finalDst, nil
	}
	if C.ZSTD_getErrorCode(result) != 0 {
		return dst, mapZstdError(result, "compression", ctx)
	}
	finalDst := dst[:dstLen+compressedSize]
	finalDst = OptimizeBuffer(finalDst)
	return finalDst, nil
}

func compressInternal(cctx, cctxDict *cctxWrapper, dst, src []byte, cd *CDict, compressionLevel int, mustSucceed bool) C.size_t {
	dstHdr := (*reflect.SliceHeader)(unsafe.Pointer(&dst))
	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))

	if cd != nil {
		// Safely acquire reference to dictionary
		if !cd.acquireRef() {
			// Dictionary was released, return error
			return C.size_t(C.ZSTD_error_GENERIC) // Return generic error
		}
		defer cd.releaseRef()

		// Check again that dictionary pointer is valid
		if cd.p == nil {
			return C.size_t(C.ZSTD_error_GENERIC)
		}

		result := C.ZSTD_compress_usingCDict_wrapper(
			unsafe.Pointer(cctxDict.cctx),
			unsafe.Pointer(dstHdr.Data),
			C.size_t(cap(dst)),
			unsafe.Pointer(srcHdr.Data),
			C.size_t(len(src)),
			unsafe.Pointer(cd.p))
		// Prevent from GC'ing of dst and src during CGO call above.
		runtime.KeepAlive(dst)
		runtime.KeepAlive(src)
		if mustSucceed {
			ensureNoError("ZSTD_compress_usingCDict", result)
		}
		return result
	}
	result := C.ZSTD_compressCCtx_wrapper(
		unsafe.Pointer(cctx.cctx),
		unsafe.Pointer(dstHdr.Data),
		C.size_t(cap(dst)),
		unsafe.Pointer(srcHdr.Data),
		C.size_t(len(src)),
		C.int(compressionLevel))
	// Prevent from GC'ing of dst and src during CGO call above.
	runtime.KeepAlive(dst)
	runtime.KeepAlive(src)
	if mustSucceed {
		ensureNoError("ZSTD_compressCCtx", result)
	}
	return result
}

func compress2Internal(cctx *cctxWrapper, dst, src []byte, mustSucceed bool) C.size_t {
	dstHdr := (*reflect.SliceHeader)(unsafe.Pointer(&dst))
	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))

	result := C.ZSTD_compress2_wrapper(
		unsafe.Pointer(cctx.cctx),
		unsafe.Pointer(dstHdr.Data),
		C.size_t(cap(dst)),
		unsafe.Pointer(srcHdr.Data),
		C.size_t(len(src)))
	// Prevent from GC'ing of dst and src during CGO call above.
	runtime.KeepAlive(dst)
	runtime.KeepAlive(src)
	if mustSucceed {
		ensureNoError("ZSTD_compress2", result)
	}
	return result
}

// Decompress appends decompressed src to dst and returns the result.
func Decompress(dst, src []byte) ([]byte, error) {
	return DecompressDict(dst, src, nil)
}

// DecompressDict appends decompressed src to dst and returns the result.
//
// The given dictionary dd is used for the decompression.
func DecompressDict(dst, src []byte, dd *DDict) ([]byte, error) {
	var dctx, dctxDict *dctxWrapper
	if dd == nil {
		dctx = dctxPool.Get().(*dctxWrapper)
	} else {
		dctxDict = dctxDictPool.Get().(*dctxWrapper)
	}

	var err error
	dst, err = decompress(dctx, dctxDict, dst, src, dd)

	if dd == nil {
		dctxPool.Put(dctx)
	} else {
		dctxDictPool.Put(dctxDict)
	}
	return dst, err
}

var dctxPool = &sync.Pool{
	New: newDCtx,
}

var dctxDictPool = &sync.Pool{
	New: newDCtx,
}

func newDCtx() interface{} {
	dctx := C.ZSTD_createDCtx()
	dw := &dctxWrapper{
		dctx: dctx,
	}
	runtime.SetFinalizer(dw, freeDCtx)
	return dw
}

func freeDCtx(dw *dctxWrapper) {
	C.ZSTD_freeDCtx(dw.dctx)
	dw.dctx = nil
}

type dctxWrapper struct {
	dctx *C.ZSTD_DCtx
}

func decompress(dctx, dctxDict *dctxWrapper, dst, src []byte, dd *DDict) ([]byte, error) {
	// Let ZSTD handle all validation including empty input and minimum size checks
	// ZSTD creates valid 6-byte frames for empty input and handles all edge cases

	dstLen := len(dst)

	// Build error context for better error reporting
	ctx := ErrorContext{
		InputSize:  len(src),
		OutputSize: cap(dst) - dstLen,
	}
	if dd != nil {
		// Note: Would need to extract dictionary ID from frame if available
		ctx.DictionaryID = 0 // Placeholder - actual extraction would be complex
	}

	if cap(dst) > dstLen {
		// Fast path - try decompressing without dst resize.
		result := decompressInternal(dctx, dctxDict, dst[dstLen:cap(dst)], src, dd)
		decompressedSize := int(result)
		if decompressedSize >= 0 {
			// Trust ZSTD's built-in memory bomb prevention
			// All OK.
			return dst[:dstLen+decompressedSize], nil
		}

		if C.ZSTD_getErrorCode(result) != C.ZSTD_error_dstSize_tooSmall {
			// Error during decompression with full context
			return dst[:dstLen], mapZstdError(result, "decompression", ctx)
		}
	}

	// Slow path - resize dst to fit decompressed data using buffer pool
	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))
	decompressBound := int(C.ZSTD_findDecompressedSize_wrapper(
		unsafe.Pointer(srcHdr.Data), C.size_t(len(src))))
	// Prevent from GC'ing of src during CGO call above.
	runtime.KeepAlive(src)
	switch uint64(decompressBound) {
	case uint64(C.ZSTD_CONTENTSIZE_UNKNOWN):
		return streamDecompress(dst, src, dd)
	case uint64(C.ZSTD_CONTENTSIZE_ERROR):
		return dst, mapZstdError(C.size_t(C.ZSTD_CONTENTSIZE_ERROR), "decompression content size detection", ctx)
	}
	decompressBound++

	requiredTotal := dstLen + decompressBound

	if cap(dst) < requiredTotal {
		// Use buffer pool for more efficient memory management
		newBuf := GetDecompressBuffer(requiredTotal)
		// Extend newBuf to have enough length for the copy
		newBuf = newBuf[:requiredTotal]
		copy(newBuf, dst[:dstLen])

		// Return old buffer to pool if it's from our pool system
		if cap(dst) > 0 && len(dst) > 0 {
			PutBuffer(dst[:0])
		}

		dst = newBuf[:dstLen]
	}

	result := decompressInternal(dctx, dctxDict, dst[dstLen:dstLen+decompressBound], src, dd)
	decompressedSize := int(result)
	if decompressedSize >= 0 {
		// Trust ZSTD's built-in memory bomb prevention
		dst = dst[:dstLen+decompressedSize]

		// Optimize buffer to reduce memory waste
		dst = OptimizeBuffer(dst)

		return dst, nil
	}

	// Error during decompression with full context
	return dst[:dstLen], mapZstdError(result, "decompression", ctx)
}

func decompressInternal(dctx, dctxDict *dctxWrapper, dst, src []byte, dd *DDict) C.size_t {
	var (
		dstHdr = (*reflect.SliceHeader)(unsafe.Pointer(&dst))
		srcHdr = (*reflect.SliceHeader)(unsafe.Pointer(&src))
		n      C.size_t
	)
	if dd != nil {
		// Safely acquire reference to dictionary
		if !dd.acquireRef() {
			// Dictionary was released, return error
			return C.size_t(C.ZSTD_error_GENERIC)
		}
		defer dd.releaseRef()

		// Check again that dictionary pointer is valid
		if dd.p == nil {
			return C.size_t(C.ZSTD_error_GENERIC)
		}

		n = C.ZSTD_decompress_usingDDict_wrapper(
			unsafe.Pointer(dctxDict.dctx),
			unsafe.Pointer(dstHdr.Data),
			C.size_t(cap(dst)),
			unsafe.Pointer(srcHdr.Data),
			C.size_t(len(src)),
			unsafe.Pointer(dd.p))
	} else {
		n = C.ZSTD_decompressDCtx_wrapper(
			unsafe.Pointer(dctx.dctx),
			unsafe.Pointer(dstHdr.Data),
			C.size_t(cap(dst)),
			unsafe.Pointer(srcHdr.Data),
			C.size_t(len(src)))
	}
	// Prevent from GC'ing of dst and src during CGO call above.
	runtime.KeepAlive(dst)
	runtime.KeepAlive(src)
	return n
}

func ensureNoError(funcName string, result C.size_t) {
	if zstdIsError(result) {
		// Use the new error mapping system for better error information
		ctx := ErrorContext{} // Basic context - caller should provide more if needed
		err := mapZstdError(result, funcName, ctx)
		panic(fmt.Errorf("BUG: unexpected error in %s: %w", funcName, err))
	}
}

func zstdIsError(result C.size_t) bool {
	if int(result) >= 0 {
		// Fast path - avoid calling C function.
		return false
	}
	return C.ZSTD_isError(result) != 0
}

func streamDecompress(dst, src []byte, dd *DDict) ([]byte, error) {
	sd := getStreamDecompressor(dd)
	sd.dst = dst
	sd.src = src
	_, err := sd.zr.WriteTo(sd)
	dst = sd.dst

	// Trust ZSTD's built-in validation for stream decompression

	putStreamDecompressor(sd)
	return dst, err
}

type streamDecompressor struct {
	dst       []byte
	src       []byte
	srcOffset int

	zr *Reader
}

type srcReader streamDecompressor

func (sr *srcReader) Read(p []byte) (int, error) {
	sd := (*streamDecompressor)(sr)
	n := copy(p, sd.src[sd.srcOffset:])
	sd.srcOffset += n
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (sd *streamDecompressor) Write(p []byte) (int, error) {
	sd.dst = append(sd.dst, p...)
	return len(p), nil
}

func getStreamDecompressor(dd *DDict) *streamDecompressor {
	v := streamDecompressorPool.Get()
	if v == nil {
		sd := &streamDecompressor{
			zr: NewReader(nil),
		}
		v = sd
	}
	sd := v.(*streamDecompressor)
	sd.zr.Reset((*srcReader)(sd), dd)
	return sd
}

func putStreamDecompressor(sd *streamDecompressor) {
	sd.dst = nil
	sd.src = nil
	sd.srcOffset = 0
	sd.zr.Reset(nil, nil)
	streamDecompressorPool.Put(sd)
}

var streamDecompressorPool sync.Pool

package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"

#define ZDICT_STATIC_LINKING_ONLY
#include "zdict.h"

#include <stdint.h>  // for uintptr_t

// The following *_wrapper functions allow avoiding memory allocations
// durting calls from Go.
// See https://github.com/golang/go/issues/24450 .

static ZSTD_CDict* ZSTD_createCDict_wrapper(void *dictBuffer, size_t dictSize, int compressionLevel) {
	return ZSTD_createCDict((const void *)dictBuffer, dictSize, compressionLevel);
}

static ZSTD_DDict* ZSTD_createDDict_wrapper(void *dictBuffer, size_t dictSize) {
	return ZSTD_createDDict((const void *)dictBuffer, dictSize);
}

static ZSTD_CDict* ZSTD_createCDict_byReference_wrapper(void *dictBuffer, size_t dictSize, int compressionLevel) {
	return ZSTD_createCDict_byReference((const void *)dictBuffer, dictSize, compressionLevel);
}

static ZSTD_DDict* ZSTD_createDDict_byReference_wrapper(void *dictBuffer, size_t dictSize) {
	return ZSTD_createDDict_byReference((const void *)dictBuffer, dictSize);
}

// Wrapper functions for enhanced dictionary training
static size_t ZDICT_trainFromBuffer_cover_wrapper(
    void *dictBuffer, size_t dictBufferCapacity,
    const void *samplesBuffer, const size_t *samplesSizes, unsigned nbSamples,
    unsigned k, unsigned d, unsigned steps, unsigned nbThreads,
    double splitPoint, unsigned shrinkDict, unsigned shrinkDictMaxRegression,
    int compressionLevel, unsigned notificationLevel, unsigned dictID) {

    ZDICT_cover_params_t params = {0};
    params.k = k;
    params.d = d;
    params.steps = steps;
    params.nbThreads = nbThreads;
    params.splitPoint = splitPoint;
    params.shrinkDict = shrinkDict;
    params.shrinkDictMaxRegression = shrinkDictMaxRegression;
    params.zParams.compressionLevel = compressionLevel;
    params.zParams.notificationLevel = notificationLevel;
    params.zParams.dictID = dictID;

    return ZDICT_trainFromBuffer_cover(dictBuffer, dictBufferCapacity,
                                      samplesBuffer, samplesSizes, nbSamples,
                                      params);
}

static size_t ZDICT_optimizeTrainFromBuffer_cover_wrapper(
    void *dictBuffer, size_t dictBufferCapacity,
    const void *samplesBuffer, const size_t *samplesSizes, unsigned nbSamples,
    unsigned k, unsigned d, unsigned steps, unsigned nbThreads,
    double splitPoint, unsigned shrinkDict, unsigned shrinkDictMaxRegression,
    int compressionLevel, unsigned notificationLevel, unsigned dictID,
    unsigned *optimizedK, unsigned *optimizedD, unsigned *optimizedSteps) {

    ZDICT_cover_params_t params = {0};
    params.k = k;
    params.d = d;
    params.steps = steps;
    params.nbThreads = nbThreads;
    params.splitPoint = splitPoint;
    params.shrinkDict = shrinkDict;
    params.shrinkDictMaxRegression = shrinkDictMaxRegression;
    params.zParams.compressionLevel = compressionLevel;
    params.zParams.notificationLevel = notificationLevel;
    params.zParams.dictID = dictID;

    size_t result = ZDICT_optimizeTrainFromBuffer_cover(dictBuffer, dictBufferCapacity,
                                                       samplesBuffer, samplesSizes, nbSamples,
                                                       &params);

    // Return optimized parameters
    *optimizedK = params.k;
    *optimizedD = params.d;
    *optimizedSteps = params.steps;

    return result;
}

static size_t ZDICT_trainFromBuffer_fastCover_wrapper(
    void *dictBuffer, size_t dictBufferCapacity,
    const void *samplesBuffer, const size_t *samplesSizes, unsigned nbSamples,
    unsigned k, unsigned d, unsigned f, unsigned steps, unsigned nbThreads,
    double splitPoint, unsigned accel, unsigned shrinkDict, unsigned shrinkDictMaxRegression,
    int compressionLevel, unsigned notificationLevel, unsigned dictID) {

    ZDICT_fastCover_params_t params = {0};
    params.k = k;
    params.d = d;
    params.f = f;
    params.steps = steps;
    params.nbThreads = nbThreads;
    params.splitPoint = splitPoint;
    params.accel = accel;
    params.shrinkDict = shrinkDict;
    params.shrinkDictMaxRegression = shrinkDictMaxRegression;
    params.zParams.compressionLevel = compressionLevel;
    params.zParams.notificationLevel = notificationLevel;
    params.zParams.dictID = dictID;

    return ZDICT_trainFromBuffer_fastCover(dictBuffer, dictBufferCapacity,
                                          samplesBuffer, samplesSizes, nbSamples,
                                          params);
}

static size_t ZDICT_optimizeTrainFromBuffer_fastCover_wrapper(
    void *dictBuffer, size_t dictBufferCapacity,
    const void *samplesBuffer, const size_t *samplesSizes, unsigned nbSamples,
    unsigned k, unsigned d, unsigned f, unsigned steps, unsigned nbThreads,
    double splitPoint, unsigned accel, unsigned shrinkDict, unsigned shrinkDictMaxRegression,
    int compressionLevel, unsigned notificationLevel, unsigned dictID,
    unsigned *optimizedK, unsigned *optimizedD, unsigned *optimizedF, unsigned *optimizedSteps, unsigned *optimizedAccel) {

    ZDICT_fastCover_params_t params = {0};
    params.k = k;
    params.d = d;
    params.f = f;
    params.steps = steps;
    params.nbThreads = nbThreads;
    params.splitPoint = splitPoint;
    params.accel = accel;
    params.shrinkDict = shrinkDict;
    params.shrinkDictMaxRegression = shrinkDictMaxRegression;
    params.zParams.compressionLevel = compressionLevel;
    params.zParams.notificationLevel = notificationLevel;
    params.zParams.dictID = dictID;

    size_t result = ZDICT_optimizeTrainFromBuffer_fastCover(dictBuffer, dictBufferCapacity,
                                                           samplesBuffer, samplesSizes, nbSamples,
                                                           &params);

    // Return optimized parameters
    *optimizedK = params.k;
    *optimizedD = params.d;
    *optimizedF = params.f;
    *optimizedSteps = params.steps;
    *optimizedAccel = params.accel;

    return result;
}

static size_t ZDICT_trainFromBuffer_legacy_wrapper(
    void *dictBuffer, size_t dictBufferCapacity,
    const void *samplesBuffer, const size_t *samplesSizes, unsigned nbSamples,
    unsigned selectivityLevel, int compressionLevel, unsigned notificationLevel, unsigned dictID) {

    ZDICT_legacy_params_t params = {0};
    params.selectivityLevel = selectivityLevel;
    params.zParams.compressionLevel = compressionLevel;
    params.zParams.notificationLevel = notificationLevel;
    params.zParams.dictID = dictID;

    return ZDICT_trainFromBuffer_legacy(dictBuffer, dictBufferCapacity,
                                       samplesBuffer, samplesSizes, nbSamples,
                                       params);
}

*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

const minDictLen = C.ZDICT_DICTSIZE_MIN

// BuildDict returns dictionary built from the given samples.
//
// The resulting dictionary size will be close to desiredDictLen.
//
// The returned dictionary may be passed to NewCDict* and NewDDict.
func BuildDict(samples [][]byte, desiredDictLen int) []byte {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}
	dict := make([]byte, desiredDictLen)

	// Calculate the total samples size.
	samplesBufLen := 0
	for _, sample := range samples {
		if len(sample) == 0 {
			// Skip empty samples.
			continue
		}
		samplesBufLen += len(sample)
	}

	// Construct flat samplesBuf and samplesSizes.
	samplesBuf := make([]byte, 0, samplesBufLen)
	samplesSizes := make([]C.size_t, 0, len(samples))
	for _, sample := range samples {
		samplesBuf = append(samplesBuf, sample...)
		samplesSizes = append(samplesSizes, C.size_t(len(sample)))
	}

	// Add fake samples if the original samples are too small.
	minSamplesBufLen := int(C.ZDICT_CONTENTSIZE_MIN)
	if minSamplesBufLen < minDictLen {
		minSamplesBufLen = minDictLen
	}
	for samplesBufLen < minSamplesBufLen {
		fakeSample := []byte(fmt.Sprintf("this is a fake sample %d", samplesBufLen))
		samplesBuf = append(samplesBuf, fakeSample...)
		samplesSizes = append(samplesSizes, C.size_t(len(fakeSample)))
		samplesBufLen += len(fakeSample)
	}

	// Run ZDICT_trainFromBuffer under lock, since it looks like it
	// is unsafe for concurrent usage (it just randomly crashes).
	// TODO: remove this restriction.

	buildDictLock.Lock()
	result := C.ZDICT_trainFromBuffer(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)))
	buildDictLock.Unlock()
	if C.ZDICT_isError(result) != 0 {
		// Return empty dictionary, since the original samples are too small.
		return nil
	}

	dictLen := int(result)
	return dict[:dictLen]
}

var buildDictLock sync.Mutex

// CDict is a dictionary used for compression.
//
// A single CDict may be re-used in concurrently running goroutines.
type CDict struct {
	p                *C.ZSTD_CDict
	compressionLevel int
	refCount         int64 // atomic reference counter
	released         int64 // atomic flag indicating if dictionary is released
	generation       int64 // atomic generation counter to prevent ABA problem
}

// NewCDict creates new CDict from the given dict.
//
// Call Release when the returned dict is no longer used.
func NewCDict(dict []byte) (*CDict, error) {
	return NewCDictLevel(dict, DefaultCompressionLevel)
}

// NewCDictLevel creates new CDict from the given dict
// using the given compressionLevel.
//
// Call Release when the returned dict is no longer used.
func NewCDictLevel(dict []byte, compressionLevel int) (*CDict, error) {
	if len(dict) == 0 {
		return nil, fmt.Errorf("dict cannot be empty")
	}

	cd := &CDict{
		p: C.ZSTD_createCDict_wrapper(
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict)),
			C.int(compressionLevel)),
		compressionLevel: compressionLevel,
		refCount:         1, // Start with 1 reference
		released:         0,
		generation:       0, // Start with generation 0
	}
	// Prevent from GC'ing of dict during CGO call above.
	runtime.KeepAlive(dict)
	runtime.SetFinalizer(cd, freeCDict)
	return cd, nil
}

// acquireRef safely acquires a reference to the dictionary.
// Returns true if successful, false if dictionary is already released.
// Uses generation counter to prevent ABA problem where dictionary is freed and reallocated.
func (cd *CDict) acquireRef() bool {
	for {
		// Read generation first to establish ordering
		generation := atomic.LoadInt64(&cd.generation)

		if atomic.LoadInt64(&cd.released) != 0 {
			return false // Dictionary already released
		}

		oldCount := atomic.LoadInt64(&cd.refCount)
		if oldCount <= 0 {
			return false // Invalid reference count
		}

		if atomic.CompareAndSwapInt64(&cd.refCount, oldCount, oldCount+1) {
			// Verify generation hasn't changed (prevents ABA problem)
			if atomic.LoadInt64(&cd.generation) != generation {
				// Dictionary was released and possibly reallocated, undo
				atomic.AddInt64(&cd.refCount, -1)
				return false
			}

			// Double-check released flag after incrementing
			if atomic.LoadInt64(&cd.released) != 0 {
				// Dictionary was released after we incremented, undo
				atomic.AddInt64(&cd.refCount, -1)
				return false
			}
			return true
		}
	}
}

// releaseRef safely releases a reference to the dictionary.
// Frees the dictionary if this was the last reference.
func (cd *CDict) releaseRef() {
	newCount := atomic.AddInt64(&cd.refCount, -1)
	if newCount == 0 {
		// Last reference - free the dictionary
		if cd.p != nil {
			result := C.ZSTD_freeCDict(cd.p)
			ensureNoError("ZSTD_freeCDict", result)
			cd.p = nil
		}
	} else if newCount < 0 {
		panic("BUG: CDict reference count went negative")
	}
}

// Release releases resources occupied by cd.
//
// cd cannot be used after the release.
func (cd *CDict) Release() {
	if cd == nil {
		return
	}
	// Mark as released to prevent new references
	if !atomic.CompareAndSwapInt64(&cd.released, 0, 1) {
		return // Already released
	}
	// Increment generation to prevent ABA problem
	atomic.AddInt64(&cd.generation, 1)
	// Release our initial reference
	cd.releaseRef()
}

func freeCDict(v interface{}) {
	v.(*CDict).Release()
}

// DDict is a dictionary used for decompression.
//
// A single DDict may be re-used in concurrently running goroutines.
type DDict struct {
	p          *C.ZSTD_DDict
	refCount   int64 // atomic reference counter
	released   int64 // atomic flag indicating if dictionary is released
	generation int64 // atomic generation counter to prevent ABA problem
}

// NewDDict creates new DDict from the given dict.
//
// Call Release when the returned dict is no longer needed.
func NewDDict(dict []byte) (*DDict, error) {
	if len(dict) == 0 {
		return nil, fmt.Errorf("dict cannot be empty")
	}

	dd := &DDict{
		p: C.ZSTD_createDDict_wrapper(
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict))),
		refCount:   1, // Start with 1 reference
		released:   0,
		generation: 0, // Start with generation 0
	}
	// Prevent from GC'ing of dict during CGO call above.
	runtime.KeepAlive(dict)
	runtime.SetFinalizer(dd, freeDDict)
	return dd, nil
}

// acquireRef safely acquires a reference to the dictionary.
// Returns true if successful, false if dictionary is already released.
// Uses generation counter to prevent ABA problem where dictionary is freed and reallocated.
func (dd *DDict) acquireRef() bool {
	for {
		// Read generation first to establish ordering
		generation := atomic.LoadInt64(&dd.generation)

		if atomic.LoadInt64(&dd.released) != 0 {
			return false // Dictionary already released
		}

		oldCount := atomic.LoadInt64(&dd.refCount)
		if oldCount <= 0 {
			return false // Invalid reference count
		}

		if atomic.CompareAndSwapInt64(&dd.refCount, oldCount, oldCount+1) {
			// Verify generation hasn't changed (prevents ABA problem)
			if atomic.LoadInt64(&dd.generation) != generation {
				// Dictionary was released and possibly reallocated, undo
				atomic.AddInt64(&dd.refCount, -1)
				return false
			}

			// Double-check released flag after incrementing
			if atomic.LoadInt64(&dd.released) != 0 {
				// Dictionary was released after we incremented, undo
				atomic.AddInt64(&dd.refCount, -1)
				return false
			}
			return true
		}
	}
}

// releaseRef safely releases a reference to the dictionary.
// Frees the dictionary if this was the last reference.
func (dd *DDict) releaseRef() {
	newCount := atomic.AddInt64(&dd.refCount, -1)
	if newCount == 0 {
		// Last reference - free the dictionary
		if dd.p != nil {
			result := C.ZSTD_freeDDict(dd.p)
			ensureNoError("ZSTD_freeDDict", result)
			dd.p = nil
		}
	} else if newCount < 0 {
		panic("BUG: DDict reference count went negative")
	}
}

// Release releases resources occupied by dd.
//
// dd cannot be used after the release.
func (dd *DDict) Release() {
	if dd == nil {
		return
	}
	// Mark as released to prevent new references
	if !atomic.CompareAndSwapInt64(&dd.released, 0, 1) {
		return // Already released
	}
	// Increment generation to prevent ABA problem
	atomic.AddInt64(&dd.generation, 1)
	// Release our initial reference
	dd.releaseRef()
}

func freeDDict(v interface{}) {
	v.(*DDict).Release()
}

// NewCDictByRef creates new CDict from the given dict without copying the dict data.
//
// The dict data must remain valid throughout the lifetime of the returned CDict.
//
// This is more memory-efficient than NewCDict when the dict data is already
// stored elsewhere and doesn't need to be copied.
//
// Call Release when the returned dict is no longer used.
func NewCDictByRef(dict []byte) (*CDict, error) {
	return NewCDictByRefLevel(dict, DefaultCompressionLevel)
}

// NewCDictByRefLevel creates new CDict from the given dict
// using the given compressionLevel without copying the dict data.
//
// The dict data must remain valid throughout the lifetime of the returned CDict.
//
// This is more memory-efficient than NewCDictLevel when the dict data is already
// stored elsewhere and doesn't need to be copied.
//
// Call Release when the returned dict is no longer used.
func NewCDictByRefLevel(dict []byte, compressionLevel int) (*CDict, error) {
	if len(dict) == 0 {
		return nil, fmt.Errorf("dict cannot be empty")
	}

	cd := &CDict{
		p: C.ZSTD_createCDict_byReference_wrapper(
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict)),
			C.int(compressionLevel)),
		compressionLevel: compressionLevel,
		refCount:         1, // Start with 1 reference
		released:         0,
		generation:       0, // Start with generation 0
	}
	// Prevent from GC'ing of dict during CGO call above.
	// IMPORTANT: The caller must ensure dict remains valid for the lifetime of the CDict
	runtime.KeepAlive(dict)
	runtime.SetFinalizer(cd, freeCDict)
	return cd, nil
}

// NewDDictByRef creates new DDict from the given dict without copying the dict data.
//
// The dict data must remain valid throughout the lifetime of the returned DDict.
//
// This is more memory-efficient than NewDDict when the dict data is already
// stored elsewhere and doesn't need to be copied.
//
// Call Release when the returned dict is no longer needed.
func NewDDictByRef(dict []byte) (*DDict, error) {
	if len(dict) == 0 {
		return nil, fmt.Errorf("dict cannot be empty")
	}

	dd := &DDict{
		p: C.ZSTD_createDDict_byReference_wrapper(
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict))),
		refCount:   1, // Start with 1 reference
		released:   0,
		generation: 0, // Start with generation 0
	}
	// Prevent from GC'ing of dict during CGO call above.
	// IMPORTANT: The caller must ensure dict remains valid for the lifetime of the DDict
	runtime.KeepAlive(dict)
	runtime.SetFinalizer(dd, freeDDict)
	return dd, nil
}


// ============================================================================
// Enhanced Dictionary Training with COVER algorithms
// (merged from dict_training.go)
// ============================================================================

// CoverParams contains parameters for the COVER dictionary training algorithm.
//
// COVER algorithm parameters control how the algorithm analyzes the training corpus
// to build an optimal dictionary. The algorithm finds frequent segments (k-length)
// that are composed of frequent sub-patterns (d-mers).
//
// PARAMETER GUIDELINES:
// - K (segment size): [16, 2048+] - larger values find longer patterns but use more memory
// - D (d-mer size): [6, 16] and D <= K - smaller values are faster but may miss patterns
// - Steps: optimization iterations (0=default 40) - more steps find better parameters but take longer
// - Threads: parallel training (0=single-threaded) - speeds up training on multi-core systems
// - SplitPoint: [0.0, 1.0] fraction for training vs testing (0=default 1.0) - enables validation
type CoverParams struct {
	// K: Segment size constraint: 0 < K. Reasonable range: [16, 2048+]
	// Controls the length of segments that COVER looks for in the training data.
	// Larger values can find longer patterns but require more memory.
	K uint32

	// D: D-mer size constraint: 0 < D <= K. Reasonable range: [6, 16]
	// Controls the size of sub-patterns used to identify segments.
	// Smaller values are faster but may miss some patterns.
	D uint32

	// Steps: Number of optimization steps. 0 means default (40).
	// Only used by optimization functions. Higher values check more parameter combinations
	// but take longer to complete.
	Steps uint32

	// Threads: Number of threads for parallel processing. 0 means single-threaded.
	// Only used by optimization functions. Speeds up training on multi-core systems.
	Threads uint32

	// SplitPoint: Percentage of samples for training vs testing. Range: [0.0, 1.0]
	// 0 means default (1.0). When < 1.0, enables validation by splitting the corpus.
	// First (samples * SplitPoint) used for training, rest for testing compression ratio.
	SplitPoint float64

	// ShrinkDict: Enable dictionary shrinking. 0=disabled, 1=enabled.
	// When enabled, trains multiple dictionary sizes and selects the smallest one
	// that doesn't hurt compression ratio too much.
	ShrinkDict uint32

	// ShrinkDictMaxRegression: Maximum regression percentage for dictionary shrinking.
	// Allows smaller dictionaries that are at most this percentage worse than the largest.
	ShrinkDictMaxRegression uint32

	// CompressionLevel: Compression level for testing during training.
	// Affects the quality assessment during parameter optimization.
	CompressionLevel int

	// NotificationLevel: Verbosity level for training output (0=silent, higher=more verbose)
	NotificationLevel uint32

	// DictID: Dictionary ID to embed in the dictionary (0=auto-generate)
	DictID uint32
}

// FastCoverParams contains parameters for the FastCover dictionary training algorithm.
//
// FastCover is a faster version of COVER that uses a frequency array to speed up
// the segment selection process. It provides similar quality to COVER but with
// significantly improved training performance.
//
// ADDITIONAL PARAMETERS vs COVER:
// - F: Log of frequency array size [1, 31] - larger values are more accurate but use more memory
// - Accel: Acceleration level [1, 10] - higher values are faster but less accurate
type FastCoverParams struct {
	// K: Segment size constraint: 0 < K. Reasonable range: [16, 2048+]
	K uint32

	// D: D-mer size constraint: 0 < D <= K. Reasonable range: [6, 16]
	D uint32

	// F: Log of frequency array size constraint: 0 < F <= 31. Default: 20
	// Controls memory vs accuracy tradeoff. Larger values are more accurate but use more memory.
	// Memory usage is approximately 6 * 2^F bytes.
	F uint32

	// Steps: Number of optimization steps. 0 means default (40).
	Steps uint32

	// Threads: Number of threads. 0 means single-threaded.
	Threads uint32

	// SplitPoint: Training vs testing split. Range: [0.0, 1.0]. Default: 0.75
	SplitPoint float64

	// Accel: Acceleration level constraint: 0 < Accel <= 10. Default: 1
	// Higher values trade accuracy for speed.
	Accel uint32

	// ShrinkDict: Enable dictionary shrinking. 0=disabled, 1=enabled.
	ShrinkDict uint32

	// ShrinkDictMaxRegression: Maximum regression percentage for shrinking.
	ShrinkDictMaxRegression uint32

	// CompressionLevel: Compression level for testing.
	CompressionLevel int

	// NotificationLevel: Verbosity level (0=silent).
	NotificationLevel uint32

	// DictID: Dictionary ID (0=auto-generate).
	DictID uint32
}

// LegacyParams contains parameters for the legacy dictionary training algorithm.
//
// This algorithm is provided for compatibility with older ZSTD versions.
// For new applications, prefer COVER or FastCover algorithms.
type LegacyParams struct {
	// SelectivityLevel: Controls how selective the algorithm is when choosing dictionary content.
	// 0 means default. Larger values select more content, resulting in larger dictionaries.
	SelectivityLevel uint32

	// CompressionLevel: Compression level for testing.
	CompressionLevel int

	// NotificationLevel: Verbosity level (0=silent).
	NotificationLevel uint32

	// DictID: Dictionary ID (0=auto-generate).
	DictID uint32
}

// CoverResult contains the results of COVER algorithm training including optimized parameters.
type CoverResult struct {
	// Dictionary: The trained dictionary data
	Dictionary []byte

	// OptimizedParams: The parameters that were determined to be optimal
	OptimizedParams CoverParams
}

// FastCoverResult contains the results of FastCover algorithm training including optimized parameters.
type FastCoverResult struct {
	// Dictionary: The trained dictionary data
	Dictionary []byte

	// OptimizedParams: The parameters that were determined to be optimal
	OptimizedParams FastCoverParams
}

// BuildDictCover builds a dictionary using the COVER algorithm with specified parameters.
//
// COVER algorithm finds frequent segments in the training corpus and uses them to build
// a high-quality dictionary. It requires manual parameter tuning but produces excellent
// compression ratios when parameters are optimal for the data.
//
// MEMORY USAGE: Approximately 9 bytes per input byte
//
// PERFORMANCE: Slower than FastCover but potentially higher quality
//
// PARAMETERS:
// - samples: Training corpus - should contain thousands of similar small files
// - desiredDictLen: Target dictionary size (will be close to this size)
// - params: COVER algorithm parameters
//
// RETURNS:
// - Dictionary data ready for use with ZSTD compression
// - Error if training fails (usually due to insufficient or inappropriate training data)
func BuildDictCover(samples [][]byte, desiredDictLen int, params CoverParams) ([]byte, error) {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}

	samplesBuf, samplesSizes, err := prepareSamples(samples, desiredDictLen)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare samples: %w", err)
	}

	dict := make([]byte, desiredDictLen)

	buildDictLock.Lock()
	result := C.ZDICT_trainFromBuffer_cover_wrapper(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)),
		C.unsigned(params.K),
		C.unsigned(params.D),
		C.unsigned(params.Steps),
		C.unsigned(params.Threads),
		C.double(params.SplitPoint),
		C.unsigned(params.ShrinkDict),
		C.unsigned(params.ShrinkDictMaxRegression),
		C.int(params.CompressionLevel),
		C.unsigned(params.NotificationLevel),
		C.unsigned(params.DictID))
	buildDictLock.Unlock()

	if C.ZDICT_isError(result) != 0 {
		return nil, fmt.Errorf("COVER training failed: %s",
			C.GoString(C.ZDICT_getErrorName(result)))
	}

	dictLen := int(result)
	return dict[:dictLen], nil
}

// BuildDictCoverOptimized builds a dictionary using COVER algorithm with automatic parameter optimization.
//
// This function tries many parameter combinations and automatically selects the best ones
// based on compression ratio testing. It's more convenient than manual parameter tuning
// but takes longer to complete.
//
// OPTIMIZATION PROCESS:
// - If params.K is 0, tries multiple K values in [50, 2000]
// - If params.D is 0, tries D values {6, 8}
// - If params.Steps is 0, uses default (40)
// - Tests compression ratio with each combination
// - Returns dictionary built with optimal parameters
//
// MEMORY USAGE: Approximately 8 bytes per input byte + 5 bytes per input byte per thread
//
// PARAMETERS:
// - samples: Training corpus
// - desiredDictLen: Target dictionary size
// - params: Base parameters (0 values will be optimized)
//
// RETURNS:
// - CoverResult containing dictionary and optimized parameters
// - Error if optimization fails
func BuildDictCoverOptimized(samples [][]byte, desiredDictLen int, params CoverParams) (*CoverResult, error) {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}

	samplesBuf, samplesSizes, err := prepareSamples(samples, desiredDictLen)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare samples: %w", err)
	}

	dict := make([]byte, desiredDictLen)
	var optimizedK, optimizedD, optimizedSteps C.unsigned

	buildDictLock.Lock()
	result := C.ZDICT_optimizeTrainFromBuffer_cover_wrapper(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)),
		C.unsigned(params.K),
		C.unsigned(params.D),
		C.unsigned(params.Steps),
		C.unsigned(params.Threads),
		C.double(params.SplitPoint),
		C.unsigned(params.ShrinkDict),
		C.unsigned(params.ShrinkDictMaxRegression),
		C.int(params.CompressionLevel),
		C.unsigned(params.NotificationLevel),
		C.unsigned(params.DictID),
		&optimizedK,
		&optimizedD,
		&optimizedSteps)
	buildDictLock.Unlock()

	if C.ZDICT_isError(result) != 0 {
		return nil, fmt.Errorf("COVER optimization failed: %s",
			C.GoString(C.ZDICT_getErrorName(result)))
	}

	dictLen := int(result)

	// Update params with optimized values
	optimizedParams := params
	optimizedParams.K = uint32(optimizedK)
	optimizedParams.D = uint32(optimizedD)
	optimizedParams.Steps = uint32(optimizedSteps)

	return &CoverResult{
		Dictionary:      dict[:dictLen],
		OptimizedParams: optimizedParams,
	}, nil
}

// BuildDictFastCover builds a dictionary using the FastCover algorithm with specified parameters.
//
// FastCover is a faster version of COVER that uses a frequency array to accelerate
// segment selection. It provides similar quality to COVER but with significantly
// better training performance.
//
// MEMORY USAGE: 6 * 2^F bytes (where F is the frequency array log size)
//
// PERFORMANCE: Much faster than COVER, especially for large training corpora
//
// RECOMMENDED FOR: Most use cases where training time matters
//
// PARAMETERS:
// - samples: Training corpus
// - desiredDictLen: Target dictionary size
// - params: FastCover algorithm parameters
//
// RETURNS:
// - Dictionary data ready for use
// - Error if training fails
func BuildDictFastCover(samples [][]byte, desiredDictLen int, params FastCoverParams) ([]byte, error) {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}

	samplesBuf, samplesSizes, err := prepareSamples(samples, desiredDictLen)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare samples: %w", err)
	}

	dict := make([]byte, desiredDictLen)

	buildDictLock.Lock()
	result := C.ZDICT_trainFromBuffer_fastCover_wrapper(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)),
		C.unsigned(params.K),
		C.unsigned(params.D),
		C.unsigned(params.F),
		C.unsigned(params.Steps),
		C.unsigned(params.Threads),
		C.double(params.SplitPoint),
		C.unsigned(params.Accel),
		C.unsigned(params.ShrinkDict),
		C.unsigned(params.ShrinkDictMaxRegression),
		C.int(params.CompressionLevel),
		C.unsigned(params.NotificationLevel),
		C.unsigned(params.DictID))
	buildDictLock.Unlock()

	if C.ZDICT_isError(result) != 0 {
		return nil, fmt.Errorf("FastCover training failed: %s",
			C.GoString(C.ZDICT_getErrorName(result)))
	}

	dictLen := int(result)
	return dict[:dictLen], nil
}

// BuildDictFastCoverOptimized builds a dictionary using FastCover with automatic parameter optimization.
//
// This is the recommended function for most use cases. It combines the speed of FastCover
// with automatic parameter optimization to achieve excellent compression ratios without
// manual parameter tuning.
//
// OPTIMIZATION PROCESS:
// - If params.K is 0, tries multiple K values in [50, 2000]
// - If params.D is 0, tries D values {6, 8}
// - If params.F is 0, uses default (20)
// - If params.Accel is 0, uses default (1)
// - Tests compression ratio and selects optimal combination
//
// MEMORY USAGE: 6 * 2^F bytes per thread (where F is frequency array log size)
//
// RECOMMENDED: This is the best choice for most applications
//
// PARAMETERS:
// - samples: Training corpus (ideally 1000+ samples)
// - desiredDictLen: Target dictionary size (typically ~100KB)
// - params: Base parameters (0 values will be optimized)
//
// RETURNS:
// - FastCoverResult containing dictionary and optimized parameters
// - Error if optimization fails
func BuildDictFastCoverOptimized(samples [][]byte, desiredDictLen int, params FastCoverParams) (*FastCoverResult, error) {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}

	samplesBuf, samplesSizes, err := prepareSamples(samples, desiredDictLen)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare samples: %w", err)
	}

	dict := make([]byte, desiredDictLen)
	var optimizedK, optimizedD, optimizedF, optimizedSteps, optimizedAccel C.unsigned

	buildDictLock.Lock()
	result := C.ZDICT_optimizeTrainFromBuffer_fastCover_wrapper(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)),
		C.unsigned(params.K),
		C.unsigned(params.D),
		C.unsigned(params.F),
		C.unsigned(params.Steps),
		C.unsigned(params.Threads),
		C.double(params.SplitPoint),
		C.unsigned(params.Accel),
		C.unsigned(params.ShrinkDict),
		C.unsigned(params.ShrinkDictMaxRegression),
		C.int(params.CompressionLevel),
		C.unsigned(params.NotificationLevel),
		C.unsigned(params.DictID),
		&optimizedK,
		&optimizedD,
		&optimizedF,
		&optimizedSteps,
		&optimizedAccel)
	buildDictLock.Unlock()

	if C.ZDICT_isError(result) != 0 {
		return nil, fmt.Errorf("FastCover optimization failed: %s",
			C.GoString(C.ZDICT_getErrorName(result)))
	}

	dictLen := int(result)

	// Update params with optimized values
	optimizedParams := params
	optimizedParams.K = uint32(optimizedK)
	optimizedParams.D = uint32(optimizedD)
	optimizedParams.F = uint32(optimizedF)
	optimizedParams.Steps = uint32(optimizedSteps)
	optimizedParams.Accel = uint32(optimizedAccel)

	return &FastCoverResult{
		Dictionary:      dict[:dictLen],
		OptimizedParams: optimizedParams,
	}, nil
}

// BuildDictLegacy builds a dictionary using the legacy algorithm.
//
// This function is provided for compatibility with older ZSTD versions.
// For new applications, prefer BuildDictFastCoverOptimized() which provides
// better quality and performance.
//
// PARAMETERS:
// - samples: Training corpus
// - desiredDictLen: Target dictionary size
// - params: Legacy algorithm parameters
//
// RETURNS:
// - Dictionary data
// - Error if training fails
func BuildDictLegacy(samples [][]byte, desiredDictLen int, params LegacyParams) ([]byte, error) {
	if desiredDictLen < minDictLen {
		desiredDictLen = minDictLen
	}

	samplesBuf, samplesSizes, err := prepareSamples(samples, desiredDictLen)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare samples: %w", err)
	}

	dict := make([]byte, desiredDictLen)

	buildDictLock.Lock()
	result := C.ZDICT_trainFromBuffer_legacy_wrapper(
		unsafe.Pointer(&dict[0]),
		C.size_t(len(dict)),
		unsafe.Pointer(&samplesBuf[0]),
		&samplesSizes[0],
		C.unsigned(len(samplesSizes)),
		C.unsigned(params.SelectivityLevel),
		C.int(params.CompressionLevel),
		C.unsigned(params.NotificationLevel),
		C.unsigned(params.DictID))
	buildDictLock.Unlock()

	if C.ZDICT_isError(result) != 0 {
		return nil, fmt.Errorf("Legacy training failed: %s",
			C.GoString(C.ZDICT_getErrorName(result)))
	}

	dictLen := int(result)
	return dict[:dictLen], nil
}

// prepareSamples prepares the sample data for dictionary training by flattening
// the sample array and handling edge cases like empty samples and insufficient data.
func prepareSamples(samples [][]byte, desiredDictLen int) ([]byte, []C.size_t, error) {
	if len(samples) == 0 {
		return nil, nil, fmt.Errorf("no training samples provided")
	}

	// Calculate total samples size
	samplesBufLen := 0
	for _, sample := range samples {
		if len(sample) == 0 {
			continue // Skip empty samples
		}
		samplesBufLen += len(sample)
	}

	if samplesBufLen == 0 {
		return nil, nil, fmt.Errorf("all training samples are empty")
	}

	// Construct flat samplesBuf and samplesSizes
	samplesBuf := make([]byte, 0, samplesBufLen)
	samplesSizes := make([]C.size_t, 0, len(samples))
	for _, sample := range samples {
		if len(sample) == 0 {
			continue // Skip empty samples
		}
		samplesBuf = append(samplesBuf, sample...)
		samplesSizes = append(samplesSizes, C.size_t(len(sample)))
	}

	// Add fake samples if the original samples are too small
	minSamplesBufLen := int(C.ZDICT_CONTENTSIZE_MIN)
	if minSamplesBufLen < desiredDictLen {
		minSamplesBufLen = desiredDictLen
	}

	for samplesBufLen < minSamplesBufLen {
		fakeSample := []byte(fmt.Sprintf("fake_sample_%d_padding_data_for_minimum_corpus_size", samplesBufLen))
		samplesBuf = append(samplesBuf, fakeSample...)
		samplesSizes = append(samplesSizes, C.size_t(len(fakeSample)))
		samplesBufLen += len(fakeSample)
	}

	return samplesBuf, samplesSizes, nil
}

// DefaultCoverParams returns sensible default parameters for COVER algorithm.
//
// These parameters work well for most use cases but may not be optimal for
// specific data types. Consider using optimization functions for best results.
func DefaultCoverParams() CoverParams {
	return CoverParams{
		K:                       1024,                     // Reasonable segment size
		D:                       8,                        // Good d-mer size
		Steps:                   0,                        // Use default (40)
		Threads:                 uint32(runtime.NumCPU()), // Use all CPUs
		SplitPoint:              1.0,                      // Use all data for training
		ShrinkDict:              0,                        // Disable shrinking by default
		ShrinkDictMaxRegression: 10,                       // 10% regression allowed if shrinking enabled
		CompressionLevel:        DefaultCompressionLevel,
		NotificationLevel:       0, // Silent
		DictID:                  0, // Auto-generate
	}
}

// DefaultFastCoverParams returns sensible default parameters for FastCover algorithm.
//
// These parameters provide a good balance of training speed and dictionary quality
// for most applications.
func DefaultFastCoverParams() FastCoverParams {
	return FastCoverParams{
		K:                       1024,                     // Reasonable segment size
		D:                       8,                        // Good d-mer size
		F:                       20,                       // Default frequency array size
		Steps:                   0,                        // Use default (40)
		Threads:                 uint32(runtime.NumCPU()), // Use all CPUs
		SplitPoint:              0.75,                     // 75% for training, 25% for testing
		Accel:                   1,                        // Default acceleration
		ShrinkDict:              0,                        // Disable shrinking by default
		ShrinkDictMaxRegression: 10,                       // 10% regression allowed if shrinking enabled
		CompressionLevel:        DefaultCompressionLevel,
		NotificationLevel:       0, // Silent
		DictID:                  0, // Auto-generate
	}
}

// DefaultLegacyParams returns default parameters for legacy algorithm.
func DefaultLegacyParams() LegacyParams {
	return LegacyParams{
		SelectivityLevel:  0, // Use default
		CompressionLevel:  DefaultCompressionLevel,
		NotificationLevel: 0, // Silent
		DictID:            0, // Auto-generate
	}
}

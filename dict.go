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
	refCount         int64  // atomic reference counter
	released         int64  // atomic flag indicating if dictionary is released
	generation       int64  // atomic generation counter to prevent ABA problem
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
	refCount   int64  // atomic reference counter
	released   int64  // atomic flag indicating if dictionary is released
	generation int64  // atomic generation counter to prevent ABA problem
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

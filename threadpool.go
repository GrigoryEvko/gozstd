package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"

#include <stdint.h>  // for uintptr_t

static ZSTD_threadPool* ZSTD_createThreadPool_wrapper(size_t numThreads) {
    return ZSTD_createThreadPool(numThreads);
}

static void ZSTD_freeThreadPool_wrapper(uintptr_t pool) {
    ZSTD_freeThreadPool((ZSTD_threadPool*)pool);
}

static size_t ZSTD_CCtx_refThreadPool_wrapper(uintptr_t cctx, uintptr_t pool) {
    return ZSTD_CCtx_refThreadPool((ZSTD_CCtx*)cctx, (ZSTD_threadPool*)pool);
}

*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"
)

// Global shared thread pool - automatically used when multi-threading is enabled
var (
	globalThreadPool     *C.ZSTD_threadPool
	globalThreadPoolOnce sync.Once
)

// getGlobalThreadPool returns the global shared thread pool, creating it if needed
func getGlobalThreadPool() *C.ZSTD_threadPool {
	globalThreadPoolOnce.Do(func() {
		// Create thread pool with number of CPUs
		globalThreadPool = C.ZSTD_createThreadPool_wrapper(C.size_t(runtime.NumCPU()))
	})
	return globalThreadPool
}

// useThreadPool configures the compression context to use the global thread pool
// This is called automatically when NbWorkers > 0
func useThreadPool(cs *C.ZSTD_CStream) {
	pool := getGlobalThreadPool()
	if pool != nil {
		result := C.ZSTD_CCtx_refThreadPool_wrapper(
			C.uintptr_t(uintptr(unsafe.Pointer(cs))),
			C.uintptr_t(uintptr(unsafe.Pointer(pool))))
		ensureNoError("ZSTD_CCtx_refThreadPool", result)
	}
}
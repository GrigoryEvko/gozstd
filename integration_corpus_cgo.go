package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"

#include <stdlib.h>

// Raw ZSTD compression for comparison
static size_t raw_compress(const void* src, size_t srcSize, void* dst, size_t dstCapacity, int compressionLevel) {
    return ZSTD_compress(dst, dstCapacity, src, srcSize, compressionLevel);
}

// Raw ZSTD decompression for comparison
static size_t raw_decompress(const void* src, size_t compressedSize, void* dst, size_t dstCapacity) {
    return ZSTD_decompress(dst, dstCapacity, src, compressedSize);
}

// Get decompressed size
static unsigned long long raw_getFrameContentSize(const void* src, size_t srcSize) {
    return ZSTD_getFrameContentSize(src, srcSize);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// compressRaw uses raw ZSTD C API for compression
func compressRaw(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	// Calculate worst-case output size
	dstSize := int(C.ZSTD_compressBound(C.size_t(len(src))))
	dst := make([]byte, dstSize)

	// Compress using raw C API
	compressedSize := C.raw_compress(
		unsafe.Pointer(&src[0]),
		C.size_t(len(src)),
		unsafe.Pointer(&dst[0]),
		C.size_t(dstSize),
		C.int(level),
	)

	if C.ZSTD_isError(compressedSize) != 0 {
		errCode := C.ZSTD_getErrorCode(compressedSize)
		errStr := C.ZSTD_getErrorString(errCode)
		return nil, fmt.Errorf("raw compression failed: %s", C.GoString(errStr))
	}

	return dst[:compressedSize], nil
}

// decompressRaw uses raw ZSTD C API for decompression
func decompressRaw(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	// Get decompressed size
	decompressedSize := C.raw_getFrameContentSize(
		unsafe.Pointer(&src[0]),
		C.size_t(len(src)),
	)

	if decompressedSize == C.ZSTD_CONTENTSIZE_ERROR {
		return nil, fmt.Errorf("invalid compressed data")
	}
	if decompressedSize == C.ZSTD_CONTENTSIZE_UNKNOWN {
		return nil, fmt.Errorf("unknown decompressed size")
	}

	// Allocate output buffer
	dst := make([]byte, decompressedSize)

	// Decompress
	result := C.raw_decompress(
		unsafe.Pointer(&src[0]),
		C.size_t(len(src)),
		unsafe.Pointer(&dst[0]),
		C.size_t(decompressedSize),
	)

	if C.ZSTD_isError(result) != 0 {
		errCode := C.ZSTD_getErrorCode(result)
		errStr := C.ZSTD_getErrorString(errCode)
		return nil, fmt.Errorf("raw decompression failed: %s", C.GoString(errStr))
	}

	return dst[:result], nil
}

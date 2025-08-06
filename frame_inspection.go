package gozstd

/*
#cgo CFLAGS: -O3 -I${SRCDIR}/cgo/headers

#define ZSTD_STATIC_LINKING_ONLY
#include "zstd.h"

// Wrapper functions for frame inspection
static unsigned long long ZSTD_getFrameContentSize_wrapper(void *src, size_t srcSize) {
    return ZSTD_getFrameContentSize((const void*)src, srcSize);
}

static size_t ZSTD_findFrameCompressedSize_wrapper(void *src, size_t srcSize) {
    return ZSTD_findFrameCompressedSize((const void*)src, srcSize);
}

*/
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

// FrameInfo contains information about a ZSTD frame
type FrameInfo struct {
	ContentSize    uint64 // Uncompressed size, 0 if unknown
	CompressedSize uint64 // Compressed frame size
	HasContentSize bool   // Whether content size is available
	HasChecksum    bool   // Whether frame has checksum
	DictionaryID   uint32 // Dictionary ID, 0 if none
}

// GetFrameContentSize returns the uncompressed content size from a ZSTD frame header.
// Returns the content size if available, or an error if:
// - The frame is invalid
// - Content size is unknown (not stored in frame header)
// - Input is too small to contain a valid frame header
func GetFrameContentSize(src []byte) (uint64, error) {
	if len(src) == 0 {
		return 0, fmt.Errorf("empty input")
	}

	if len(src) < 4 {
		return 0, fmt.Errorf("input too small for ZSTD frame header")
	}

	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))
	result := C.ZSTD_getFrameContentSize_wrapper(
		unsafe.Pointer(srcHdr.Data), C.size_t(len(src)))
	runtime.KeepAlive(src)

	switch uint64(result) {
	case uint64(C.ZSTD_CONTENTSIZE_ERROR):
		return 0, fmt.Errorf("invalid ZSTD frame or corrupted data")
	case uint64(C.ZSTD_CONTENTSIZE_UNKNOWN):
		return 0, fmt.Errorf("content size unknown (not stored in frame header)")
	default:
		return uint64(result), nil
	}
}

// GetFrameCompressedSize returns the compressed size of a ZSTD frame.
// This scans the frame to determine its exact compressed size, which is useful
// when processing concatenated frames or streams.
func GetFrameCompressedSize(src []byte) (uint64, error) {
	if len(src) == 0 {
		return 0, fmt.Errorf("empty input")
	}

	if len(src) < 4 {
		return 0, fmt.Errorf("input too small for ZSTD frame header")
	}

	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))
	result := C.ZSTD_findFrameCompressedSize_wrapper(
		unsafe.Pointer(srcHdr.Data), C.size_t(len(src)))
	runtime.KeepAlive(src)

	if C.ZSTD_isError(result) != 0 {
		ctx := ErrorContext{
			InputSize: len(src),
		}
		return 0, mapZstdError(result, "frame compressed size detection", ctx)
	}

	return uint64(result), nil
}

// GetDecompressedSize is a legacy alias for GetFrameContentSize.
// It attempts to get the decompressed size from the frame header.
// Returns 0 if the size is unknown or the frame is invalid.
//
// This function maintains backward compatibility by using the modern
// ZSTD_getFrameContentSize() internally but returning 0 for error/unknown
// cases to match the behavior of the deprecated ZSTD_getDecompressedSize().
func GetDecompressedSize(src []byte) uint64 {
	if len(src) == 0 {
		return 0
	}

	srcHdr := (*reflect.SliceHeader)(unsafe.Pointer(&src))
	result := C.ZSTD_getFrameContentSize_wrapper(
		unsafe.Pointer(srcHdr.Data), C.size_t(len(src)))
	runtime.KeepAlive(src)

	switch uint64(result) {
	case uint64(C.ZSTD_CONTENTSIZE_ERROR), uint64(C.ZSTD_CONTENTSIZE_UNKNOWN):
		return 0
	default:
		return uint64(result)
	}
}

// GetFrameInfo extracts comprehensive information about a ZSTD frame.
// This combines multiple frame inspection functions to provide complete frame metadata.
func GetFrameInfo(src []byte) (*FrameInfo, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	if len(src) < 4 {
		return nil, fmt.Errorf("input too small for ZSTD frame header")
	}

	info := &FrameInfo{}

	// Get content size
	contentSize, err := GetFrameContentSize(src)
	if err != nil {
		// Check if it's just unknown vs actual error
		if err.Error() == "content size unknown (not stored in frame header)" {
			info.HasContentSize = false
			info.ContentSize = 0
		} else {
			return nil, fmt.Errorf("failed to get frame info: %w", err)
		}
	} else {
		info.HasContentSize = true
		info.ContentSize = contentSize
	}

	// Get compressed size
	compressedSize, err := GetFrameCompressedSize(src)
	if err != nil {
		return nil, fmt.Errorf("failed to get compressed size: %w", err)
	}
	info.CompressedSize = compressedSize

	// Basic frame header analysis
	if len(src) >= 4 {
		// Check ZSTD magic number
		magic := uint32(src[0]) | uint32(src[1])<<8 | uint32(src[2])<<16 | uint32(src[3])<<24
		if magic != 0xFD2FB528 { // Standard ZSTD magic number
			return nil, fmt.Errorf("invalid ZSTD magic number: 0x%08x", magic)
		}

		// Extract frame header descriptor if available
		if len(src) >= 6 {
			frameHeaderDescriptor := src[4]

			// Bit 2: Checksum flag
			info.HasChecksum = (frameHeaderDescriptor & 0x04) != 0

			// Dictionary ID is more complex to extract, would need full parsing
			// For now, set to 0 (most common case)
			info.DictionaryID = 0
		}
	}

	return info, nil
}

// IsValidZSTDFrame checks if the input starts with a valid ZSTD frame.
// This is a lightweight check that only examines the frame header.
func IsValidZSTDFrame(src []byte) bool {
	if len(src) < 4 {
		return false
	}

	// Check ZSTD magic number
	magic := uint32(src[0]) | uint32(src[1])<<8 | uint32(src[2])<<16 | uint32(src[3])<<24
	return magic == 0xFD2FB528 // Standard ZSTD magic number
}

// ValidateFrameHeader performs comprehensive validation of a ZSTD frame header.
// Returns detailed error information if the frame is invalid.
func ValidateFrameHeader(src []byte) error {
	if len(src) == 0 {
		return fmt.Errorf("empty input")
	}

	if len(src) < 4 {
		return fmt.Errorf("input too small: need at least 4 bytes for magic number, got %d", len(src))
	}

	// Check magic number
	magic := uint32(src[0]) | uint32(src[1])<<8 | uint32(src[2])<<16 | uint32(src[3])<<24
	if magic != 0xFD2FB528 {
		return fmt.Errorf("invalid ZSTD magic number: expected 0xFD2FB528, got 0x%08x", magic)
	}

	// Try to get frame info to validate structure
	_, err := GetFrameInfo(src)
	if err != nil {
		return fmt.Errorf("frame validation failed: %w", err)
	}

	return nil
}

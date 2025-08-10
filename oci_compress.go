package gozstd

import (
	"io"
	"runtime"
	"sync/atomic"
)

// OCICompressionLevel defines compression levels for OCI layers
type OCICompressionLevel int

const (
	// OCILevelFastest uses level 3 for maximum speed (like nerdctl default)
	OCILevelFastest OCICompressionLevel = 3
	// OCILevelFast uses level 6 for fast compression with better ratio
	OCILevelFast OCICompressionLevel = 6
	// OCILevelDefault uses level 9 for balanced compression
	OCILevelDefault OCICompressionLevel = 9
	// OCILevelBetter uses level 12 for better compression (default for large layers)
	OCILevelBetter OCICompressionLevel = 12
	// OCILevelBest uses level 15 for best practical compression
	OCILevelBest OCICompressionLevel = 15
	// OCILevelMax uses level 19 for maximum compression
	OCILevelMax OCICompressionLevel = 19
)

// OCICompressor provides OCI-compatible compression with automatic optimization
type OCICompressor struct {
	// User-configurable settings
	MinLevel          int  // Minimum compression level (default 6)
	MaxLevel          int  // Maximum compression level (default 19)
	DefaultLevel      int  // Default level for large objects (default 12)
	AutoLevel         bool // Auto-select based on size (default true)
	MaxWorkers        int  // Maximum worker threads (0 = auto)
	StreamThreshold   int  // Size threshold for streaming in bytes (default 10MB)
	ProgressCallback  ProgressFunc // Optional progress callback
	
	// Internal state
	bytesProcessed    int64
	totalBytes        int64
}

// ProgressFunc is called during compression with progress information
type ProgressFunc func(processed, total int64)

// DefaultOCICompressor returns a compressor with sensible defaults for OCI layers
func DefaultOCICompressor() *OCICompressor {
	return &OCICompressor{
		MinLevel:        6,
		MaxLevel:        19,
		DefaultLevel:    12, // Level 12 for large objects as requested
		AutoLevel:       true,
		MaxWorkers:      0, // Auto-detect
		StreamThreshold: 10 * 1024 * 1024, // 10MB
	}
}

// GetOptimalOCILevel returns the optimal compression level for the given data size
func (oc *OCICompressor) GetOptimalOCILevel(dataSize int) int {
	if !oc.AutoLevel {
		return oc.DefaultLevel
	}
	
	// Auto-select level based on size
	var level int
	if dataSize < 512*1024 { // < 512KB
		level = 6  // Fast with decent compression
	} else if dataSize < 5*1024*1024 { // < 5MB
		level = 9  // Good balance
	} else { // >= 5MB
		level = oc.DefaultLevel // Use configured default (12 by default)
	}
	
	// Clamp to configured range
	if level < oc.MinLevel {
		level = oc.MinLevel
	}
	if level > oc.MaxLevel {
		level = oc.MaxLevel
	}
	
	return level
}

// GetOptimalWorkers returns the optimal number of workers for the given data size
func (oc *OCICompressor) GetOptimalWorkers(dataSize int) int {
	if oc.MaxWorkers > 0 {
		return oc.MaxWorkers
	}
	
	// Auto-detect based on data size and CPU
	cpuCount := runtime.NumCPU()
	
	// Don't use workers for small data
	if dataSize < 1024*1024 { // < 1MB
		return 0
	}
	
	// Scale workers with data size
	if dataSize < 10*1024*1024 { // < 10MB
		if cpuCount >= 4 {
			return 2
		}
		return 0
	}
	
	if dataSize < 100*1024*1024 { // < 100MB
		if cpuCount >= 8 {
			return 4
		} else if cpuCount >= 4 {
			return 2
		}
		return 0
	}
	
	// Large data (>= 100MB)
	if cpuCount >= 16 {
		return 8
	} else if cpuCount >= 8 {
		return 4
	} else if cpuCount >= 4 {
		return 2
	}
	return 0
}

// CompressOCILayer compresses data with OCI-compatible settings
func CompressOCILayer(dst, src []byte) []byte {
	oc := DefaultOCICompressor()
	return oc.Compress(dst, src)
}

// CompressOCILayerLevel compresses with a specific level
func CompressOCILayerLevel(dst, src []byte, level int) []byte {
	oc := DefaultOCICompressor()
	oc.AutoLevel = false
	oc.DefaultLevel = level
	return oc.Compress(dst, src)
}

// Compress compresses data using OCI-compatible settings
func (oc *OCICompressor) Compress(dst, src []byte) []byte {
	if len(src) == 0 {
		return dst
	}
	
	// Update progress tracking
	atomic.StoreInt64(&oc.totalBytes, int64(len(src)))
	atomic.StoreInt64(&oc.bytesProcessed, 0)
	
	// Get optimal parameters
	level := oc.GetOptimalOCILevel(len(src))
	workers := oc.GetOptimalWorkers(len(src))
	
	// Report initial progress
	if oc.ProgressCallback != nil {
		oc.ProgressCallback(0, int64(len(src)))
	}
	
	// Use advanced API if we need workers or special settings
	if workers > 0 {
		ctx := NewCCtx()
		defer ctx.Release()
		
		// Set compression parameters
		ctx.SetParameter(ZSTD_c_compressionLevel, level)
		ctx.SetParameter(ZSTD_c_nbWorkers, workers)
		
		// Compress
		result, err := ctx.Compress(dst, src)
		if err != nil {
			// Fallback to simple compression
			return CompressLevel(dst, src, level)
		}
		
		// Report completion
		if oc.ProgressCallback != nil {
			oc.ProgressCallback(int64(len(src)), int64(len(src)))
		}
		
		return result
	}
	
	// Simple single-threaded compression
	result := CompressLevel(dst, src, level)
	
	// Report completion
	if oc.ProgressCallback != nil {
		oc.ProgressCallback(int64(len(src)), int64(len(src)))
	}
	
	return result
}

// StreamCompress compresses from reader to writer with OCI-compatible settings
func (oc *OCICompressor) StreamCompress(dst io.Writer, src io.Reader) error {
	level := oc.DefaultLevel
	if oc.AutoLevel {
		// For streaming, we don't know size, use default
		level = oc.DefaultLevel
	}
	
	// Create writer with appropriate level
	w := NewWriterLevel(dst, level)
	defer w.Release()
	
	// Set workers if needed
	workers := oc.GetOptimalWorkers(oc.StreamThreshold)
	if workers > 0 {
		// Note: Writer doesn't expose SetParameter, would need to extend
		// For now, use single-threaded streaming
	}
	
	// Copy data with progress tracking if available
	if oc.ProgressCallback != nil {
		return oc.copyWithProgress(w, src)
	}
	
	// Simple copy
	_, err := io.Copy(w, src)
	if err != nil {
		return err
	}
	
	return w.Close()
}

// copyWithProgress copies data while reporting progress
func (oc *OCICompressor) copyWithProgress(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64
	
	for {
		n, err := src.Read(buf)
		if n > 0 {
			nw, werr := dst.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if nw != n {
				return io.ErrShortWrite
			}
			written += int64(nw)
			
			// Report progress
			if oc.ProgressCallback != nil {
				oc.ProgressCallback(written, 0) // Total unknown for stream
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	
	// Close the writer
	if closer, ok := dst.(io.Closer); ok {
		return closer.Close()
	}
	
	return nil
}

// CompressWithCustomLevel allows users to override the compression level
func CompressOCILayerWithCustomLevel(dst, src []byte, customLevel int) []byte {
	oc := DefaultOCICompressor()
	oc.AutoLevel = false
	oc.DefaultLevel = customLevel
	return oc.Compress(dst, src)
}

// Helper functions for common use cases

// CompressOCILayerFast uses lower compression for speed
func CompressOCILayerFast(dst, src []byte) []byte {
	return CompressOCILayerLevel(dst, src, int(OCILevelFast))
}

// CompressOCILayerBest uses maximum compression
func CompressOCILayerBest(dst, src []byte) []byte {
	return CompressOCILayerLevel(dst, src, int(OCILevelMax))
}

// StreamCompressOCILayer streams compression with auto-tuning
func StreamCompressOCILayer(dst io.Writer, src io.Reader) error {
	oc := DefaultOCICompressor()
	return oc.StreamCompress(dst, src)
}

// StreamCompressOCILayerLevel streams with specific level
func StreamCompressOCILayerLevel(dst io.Writer, src io.Reader, level int) error {
	oc := DefaultOCICompressor()
	oc.AutoLevel = false
	oc.DefaultLevel = level
	return oc.StreamCompress(dst, src)
}
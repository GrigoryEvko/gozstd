package gozstd

// This file consolidates:
// - oci_compress.go
// - oci_streaming.go
// - oci_progress.go

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"
)

// ===== OCI Compression =====

const (
	// OCIMediaTypeZstd is the media type for zstd-compressed OCI layers
	OCIMediaTypeZstd = "application/vnd.oci.image.layer.v1.tar+zstd"
	
	// OCIDefaultCompressionLevel is the default compression level for OCI layers (12 for large objects)
	OCIDefaultCompressionLevel = 12
	
	// OCILargeObjectThreshold is the size threshold for considering an object "large" (5MB)
	OCILargeObjectThreshold = 5 * 1024 * 1024
)

// OCICompressor provides OCI-compliant zstd compression
type OCICompressor struct {
	level           int
	enableStreaming bool
	progressHandler ProgressHandler
	mu              sync.RWMutex
}

// NewOCICompressor creates a new OCI-compliant compressor
func NewOCICompressor() *OCICompressor {
	return &OCICompressor{
		level:           OCIDefaultCompressionLevel,
		enableStreaming: true,
	}
}

// SetLevel sets the compression level
func (c *OCICompressor) SetLevel(level int) error {
	if level < MinCompressionLevel || level > MaxCompressionLevel {
		return fmt.Errorf("invalid compression level %d, must be between %d and %d",
			level, MinCompressionLevel, MaxCompressionLevel)
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	c.level = level
	return nil
}

// SetProgressHandler sets a progress handler for compression operations
func (c *OCICompressor) SetProgressHandler(handler ProgressHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progressHandler = handler
}

// Compress compresses data with OCI-compliant settings
func (c *OCICompressor) Compress(data []byte) ([]byte, error) {
	c.mu.RLock()
	level := c.level
	handler := c.progressHandler
	c.mu.RUnlock()
	
	// Auto-adjust level for large objects
	if len(data) >= OCILargeObjectThreshold && level < OCIDefaultCompressionLevel {
		level = OCIDefaultCompressionLevel
	}
	
	// Report progress if handler is set
	if handler != nil {
		handler.OnStart(int64(len(data)))
		defer handler.OnComplete()
	}
	
	// Compress with finalizers for automatic cleanup
	ctx := NewCCtx()
	runtime.SetFinalizer(ctx, (*CCtx).Release)
	defer ctx.Release()
	
	// Set OCI-compliant parameters
	ctx.SetParameter(ZSTD_c_compressionLevel, level)
	// Don't enable zstd checksum - OCI layers have their own SHA256 digests
	ctx.SetParameter(ZSTD_c_checksumFlag, 0)
	
	// Don't use dictionaries by default (not supported in OCI)
	// Don't enable rsyncable mode (may cause compatibility issues)
	
	result, err := ctx.Compress(nil, data)
	if err != nil {
		return nil, fmt.Errorf("OCI compression failed: %w", err)
	}
	
	if handler != nil {
		handler.OnProgress(int64(len(data)), int64(len(result)))
	}
	
	return result, nil
}

// CompressStream compresses a stream with OCI-compliant settings
func (c *OCICompressor) CompressStream(dst io.Writer, src io.Reader) error {
	c.mu.RLock()
	level := c.level
	handler := c.progressHandler
	c.mu.RUnlock()
	
	// Create streaming compressor with auto-cleanup
	writer := NewWriterLevel(dst, level)
	runtime.SetFinalizer(writer, (*Writer).Release)
	defer writer.Close()
	defer writer.Release()
	
	// Don't enable zstd checksum - OCI layers have their own SHA256 digests
	// writer.SetParameter(ZSTD_c_checksumFlag, 0) // TODO: Add SetParameter method to Writer
	
	// Stream with progress reporting
	buf := make([]byte, 128*1024) // 128KB buffer
	var totalRead, totalWritten int64
	
	if handler != nil {
		handler.OnStart(-1) // Unknown total size for streaming
	}
	
	for {
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := writer.Write(buf[:n])
			if werr != nil {
				return fmt.Errorf("OCI stream compression write failed: %w", werr)
			}
			
			totalRead += int64(n)
			totalWritten += int64(written)
			
			if handler != nil {
				handler.OnProgress(totalRead, totalWritten)
			}
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("OCI stream compression read failed: %w", err)
		}
	}
	
	if handler != nil {
		handler.OnComplete()
	}
	
	return nil
}

// Decompress decompresses OCI layer data
func (c *OCICompressor) Decompress(data []byte) ([]byte, error) {
	c.mu.RLock()
	handler := c.progressHandler
	c.mu.RUnlock()
	
	if handler != nil {
		handler.OnStart(int64(len(data)))
		defer handler.OnComplete()
	}
	
	result, err := Decompress(nil, data)
	if err != nil {
		return nil, fmt.Errorf("OCI decompression failed: %w", err)
	}
	
	if handler != nil {
		handler.OnProgress(int64(len(data)), int64(len(result)))
	}
	
	return result, nil
}

// DecompressStream decompresses an OCI layer stream
func (c *OCICompressor) DecompressStream(dst io.Writer, src io.Reader) error {
	c.mu.RLock()
	handler := c.progressHandler
	c.mu.RUnlock()
	
	reader := NewReader(src)
	runtime.SetFinalizer(reader, (*Reader).Release)
	defer reader.Release()
	
	if handler != nil {
		handler.OnStart(-1) // Unknown size for streaming
	}
	
	written, err := io.Copy(dst, reader)
	
	if handler != nil {
		handler.OnProgress(written, written)
		handler.OnComplete()
	}
	
	if err != nil {
		return fmt.Errorf("OCI stream decompression failed: %w", err)
	}
	
	return nil
}

// GetMediaType returns the OCI media type for zstd compression
func (c *OCICompressor) GetMediaType() string {
	return OCIMediaTypeZstd
}

// ValidateOCILayer validates that data is a valid OCI zstd layer
func ValidateOCILayer(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("data too small to be valid OCI zstd layer")
	}
	
	// Check zstd magic number
	if !IsCompressed(data) {
		return fmt.Errorf("data is not zstd compressed")
	}
	
	// Verify it can be decompressed
	decompressedSize := GetDecompressedSize(data)
	if decompressedSize == 0 {
		// Size unknown, try to decompress a small portion to validate
		testSize := 1024
		if len(data) < testSize {
			testSize = len(data)
		}
		
		_, err := Decompress(nil, data[:testSize])
		if err != nil {
			return fmt.Errorf("invalid OCI zstd layer: %w", err)
		}
	}
	
	return nil
}

// ===== OCI Streaming =====

// OCIStreamCompressor provides streaming compression for large OCI layers
type OCIStreamCompressor struct {
	writer          *Writer
	bytesWritten    int64
	bytesCompressed int64
	mu              sync.Mutex
}

// NewOCIStreamCompressor creates a new streaming compressor
func NewOCIStreamCompressor(w io.Writer, level int) (*OCIStreamCompressor, error) {
	if level == 0 {
		level = OCIDefaultCompressionLevel
	}
	
	writer := NewWriterLevel(w, level)
	
	// Don't enable zstd checksum - OCI layers have their own SHA256 digests
	// writer.SetParameter(ZSTD_c_checksumFlag, 0) // TODO: Add SetParameter method to Writer
	
	return &OCIStreamCompressor{
		writer: writer,
	}, nil
}

// Write implements io.Writer
func (sc *OCIStreamCompressor) Write(p []byte) (int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	n, err := sc.writer.Write(p)
	sc.bytesWritten += int64(n)
	
	return n, err
}

// Flush flushes any pending compressed data
func (sc *OCIStreamCompressor) Flush() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	return sc.writer.Flush()
}

// Close closes the compressor and flushes remaining data
func (sc *OCIStreamCompressor) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	err := sc.writer.Close()
	sc.writer.Release()
	
	return err
}

// Stats returns compression statistics
func (sc *OCIStreamCompressor) Stats() (written, compressed int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	return sc.bytesWritten, sc.bytesCompressed
}

// OCIStreamDecompressor provides streaming decompression for OCI layers
type OCIStreamDecompressor struct {
	reader           *Reader
	bytesRead        int64
	bytesDecompressed int64
	mu               sync.Mutex
}

// NewOCIStreamDecompressor creates a new streaming decompressor
func NewOCIStreamDecompressor(r io.Reader) *OCIStreamDecompressor {
	return &OCIStreamDecompressor{
		reader: NewReader(r),
	}
}

// Read implements io.Reader
func (sd *OCIStreamDecompressor) Read(p []byte) (int, error) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	
	n, err := sd.reader.Read(p)
	sd.bytesDecompressed += int64(n)
	
	return n, err
}

// Close closes the decompressor
func (sd *OCIStreamDecompressor) Close() error {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	
	sd.reader.Release()
	return nil
}

// Stats returns decompression statistics
func (sd *OCIStreamDecompressor) Stats() (read, decompressed int64) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	
	return sd.bytesRead, sd.bytesDecompressed
}

// ===== OCI Progress Tracking =====

// ProgressHandler handles compression/decompression progress events
type ProgressHandler interface {
	OnStart(totalBytes int64)
	OnProgress(processedBytes, outputBytes int64)
	OnComplete()
	OnError(err error)
}

// SimpleProgressHandler provides a simple progress handler implementation
type SimpleProgressHandler struct {
	totalBytes      int64
	processedBytes  int64
	outputBytes     int64
	startTime       time.Time
	lastReportTime  time.Time
	reportInterval  time.Duration
	mu              sync.Mutex
}

// NewSimpleProgressHandler creates a new simple progress handler
func NewSimpleProgressHandler() *SimpleProgressHandler {
	return &SimpleProgressHandler{
		reportInterval: time.Second,
	}
}

// OnStart is called when operation starts
func (h *SimpleProgressHandler) OnStart(totalBytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.totalBytes = totalBytes
	h.processedBytes = 0
	h.outputBytes = 0
	h.startTime = time.Now()
	h.lastReportTime = h.startTime
}

// OnProgress is called during operation
func (h *SimpleProgressHandler) OnProgress(processedBytes, outputBytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.processedBytes = processedBytes
	h.outputBytes = outputBytes
	
	// Report progress at intervals
	now := time.Now()
	if now.Sub(h.lastReportTime) >= h.reportInterval {
		h.reportProgress()
		h.lastReportTime = now
	}
}

// OnComplete is called when operation completes
func (h *SimpleProgressHandler) OnComplete() {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.reportProgress()
	
	duration := time.Since(h.startTime)
	throughput := float64(h.processedBytes) / duration.Seconds() / (1024 * 1024)
	
	var ratio float64
	if h.processedBytes > 0 {
		ratio = float64(h.outputBytes) / float64(h.processedBytes)
	}
	
	fmt.Printf("Completed: %.2f MB in %.2fs (%.2f MB/s), ratio: %.2f\n",
		float64(h.processedBytes)/(1024*1024),
		duration.Seconds(),
		throughput,
		ratio)
}

// OnError is called when an error occurs
func (h *SimpleProgressHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	fmt.Printf("Error after %d bytes: %v\n", h.processedBytes, err)
}

func (h *SimpleProgressHandler) reportProgress() {
	if h.totalBytes > 0 {
		percent := float64(h.processedBytes) * 100 / float64(h.totalBytes)
		fmt.Printf("Progress: %.1f%% (%d/%d bytes)\n",
			percent, h.processedBytes, h.totalBytes)
	} else {
		fmt.Printf("Processed: %d bytes\n", h.processedBytes)
	}
}

// ContainerdProgressHandler integrates with containerd's progress reporting
type ContainerdProgressHandler struct {
	ctx      context.Context
	callback func(int64, int64)
}

// NewContainerdProgressHandler creates a handler for containerd integration
func NewContainerdProgressHandler(ctx context.Context, callback func(int64, int64)) *ContainerdProgressHandler {
	return &ContainerdProgressHandler{
		ctx:      ctx,
		callback: callback,
	}
}

// OnStart implements ProgressHandler
func (h *ContainerdProgressHandler) OnStart(totalBytes int64) {
	if h.callback != nil {
		h.callback(0, totalBytes)
	}
}

// OnProgress implements ProgressHandler
func (h *ContainerdProgressHandler) OnProgress(processedBytes, outputBytes int64) {
	select {
	case <-h.ctx.Done():
		return
	default:
		if h.callback != nil {
			h.callback(processedBytes, outputBytes)
		}
	}
}

// OnComplete implements ProgressHandler
func (h *ContainerdProgressHandler) OnComplete() {
	// No-op for containerd
}

// OnError implements ProgressHandler
func (h *ContainerdProgressHandler) OnError(err error) {
	// Errors are typically handled by the caller in containerd
}
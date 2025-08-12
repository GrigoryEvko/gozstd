package gozstd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CompressFile compresses a file with automatic resource management
func CompressFile(srcPath, dstPath string) error {
	return CompressFileLevel(srcPath, dstPath, DefaultCompressionLevel)
}

// CompressFileLevel compresses a file with specified compression level
func CompressFileLevel(srcPath, dstPath string, level int) error {
	// Read source file
	srcData, err := ioutil.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	
	// Compress data
	compressed := CompressLevel(nil, srcData, level)
	
	// Create destination directory if needed
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Write compressed data
	if err := ioutil.WriteFile(dstPath, compressed, 0644); err != nil {
		return fmt.Errorf("failed to write compressed file: %w", err)
	}
	
	return nil
}

// DecompressFile decompresses a file with automatic resource management
func DecompressFile(srcPath, dstPath string) error {
	// Read compressed file
	compressed, err := ioutil.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read compressed file: %w", err)
	}
	
	// Decompress data
	decompressed, err := Decompress(nil, compressed)
	if err != nil {
		return fmt.Errorf("decompression failed: %w", err)
	}
	
	// Create destination directory if needed
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Write decompressed data
	if err := ioutil.WriteFile(dstPath, decompressed, 0644); err != nil {
		return fmt.Errorf("failed to write decompressed file: %w", err)
	}
	
	return nil
}

// CompressDir compresses a directory recursively
func CompressDir(srcDir, dstFile string) error {
	// This would typically create a tar.zst archive
	// For now, we'll implement a simple version
	return fmt.Errorf("directory compression not yet implemented")
}

// AutoCompress automatically detects the best compression method
func AutoCompress(data []byte) ([]byte, error) {
	// Check if data is already compressed
	if IsCompressed(data) {
		return data, nil // Already compressed
	}
	
	// Check if data is too small to benefit from compression
	if len(data) < 100 {
		return data, nil // Too small
	}
	
	// Try compression and check if it's beneficial
	compressed := Compress(nil, data)
	
	// Only use compressed if it's smaller
	if len(compressed) < len(data)*9/10 { // At least 10% reduction
		return compressed, nil
	}
	
	return data, nil // Compression not beneficial
}

// IsCompressed checks if data is already compressed (zstd format)
func IsCompressed(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// Check for zstd magic number: 0xFD2FB528
	return data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD
}

// CompressReader wraps a reader with compression
type CompressReader struct {
	reader io.Reader
	writer *Writer
	buffer *bytes.Buffer
	closed bool
	mu     sync.Mutex
}

// NewCompressReader creates a reader that compresses data on-the-fly
func NewCompressReader(r io.Reader) *CompressReader {
	buf := &bytes.Buffer{}
	return &CompressReader{
		reader: r,
		writer: NewWriter(buf),
		buffer: buf,
	}
}

// Read implements io.Reader
func (cr *CompressReader) Read(p []byte) (int, error) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	
	if cr.closed {
		return 0, io.EOF
	}
	
	// If buffer has data, return it
	if cr.buffer.Len() > 0 {
		return cr.buffer.Read(p)
	}
	
	// Read from source and compress
	temp := make([]byte, len(p)*2) // Read more to compress efficiently
	n, err := cr.reader.Read(temp)
	if n > 0 {
		if _, werr := cr.writer.Write(temp[:n]); werr != nil {
			return 0, werr
		}
		if ferr := cr.writer.Flush(); ferr != nil {
			return 0, ferr
		}
	}
	
	if err == io.EOF {
		cr.closed = true
		if cerr := cr.writer.Close(); cerr != nil {
			return 0, cerr
		}
	}
	
	// Read compressed data
	return cr.buffer.Read(p)
}

// Close closes the reader
func (cr *CompressReader) Close() error {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	
	if cr.closed {
		return nil
	}
	
	cr.closed = true
	err := cr.writer.Close()
	cr.writer.Release()
	return err
}

// DecompressReader wraps a reader with decompression
type DecompressReader struct {
	*Reader
}

// NewDecompressReader creates a reader that decompresses data on-the-fly
func NewDecompressReader(r io.Reader) *DecompressReader {
	return &DecompressReader{
		Reader: NewReader(r),
	}
}

// CompressWriter wraps a writer with compression
type CompressWriter struct {
	*Writer
}

// NewCompressWriter creates a writer that compresses data
func NewCompressWriter(w io.Writer) *CompressWriter {
	return &CompressWriter{
		Writer: NewWriter(w),
	}
}

// DecompressWriter wraps a writer with decompression
type DecompressWriter struct {
	writer io.Writer
	reader *Reader
	pipe   *io.PipeWriter
	done   chan error
}

// NewDecompressWriter creates a writer that decompresses data
func NewDecompressWriter(w io.Writer) *DecompressWriter {
	pr, pw := io.Pipe()
	dw := &DecompressWriter{
		writer: w,
		reader: NewReader(pr),
		pipe:   pw,
		done:   make(chan error, 1),
	}
	
	// Start decompression goroutine
	go func() {
		_, err := io.Copy(w, dw.reader)
		dw.done <- err
		dw.reader.Release()
	}()
	
	return dw
}

// Write implements io.Writer
func (dw *DecompressWriter) Write(p []byte) (int, error) {
	return dw.pipe.Write(p)
}

// Close closes the writer
func (dw *DecompressWriter) Close() error {
	err := dw.pipe.Close()
	if werr := <-dw.done; werr != nil && err == nil {
		err = werr
	}
	return err
}

// CompressWithContext compresses with context for cancellation
func CompressWithContext(ctx context.Context, dst, src []byte) ([]byte, error) {
	done := make(chan struct{})
	var result []byte
	
	go func() {
		result = Compress(dst, src)
		close(done)
	}()
	
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return result, nil
	}
}

// DecompressWithContext decompresses with context for cancellation
func DecompressWithContext(ctx context.Context, dst, src []byte) ([]byte, error) {
	done := make(chan struct{})
	var result []byte
	var err error
	
	go func() {
		result, err = Decompress(dst, src)
		close(done)
	}()
	
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return result, err
	}
}

// BatchCompress compresses multiple inputs in parallel
func BatchCompress(inputs [][]byte) ([][]byte, error) {
	results := make([][]byte, len(inputs))
	var wg sync.WaitGroup
	
	for i := range inputs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = Compress(nil, inputs[idx])
		}(i)
	}
	
	wg.Wait()
	
	return results, nil
}

// BatchDecompress decompresses multiple inputs in parallel
func BatchDecompress(inputs [][]byte) ([][]byte, error) {
	results := make([][]byte, len(inputs))
	errors := make([]error, len(inputs))
	var wg sync.WaitGroup
	
	for i := range inputs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = Decompress(nil, inputs[idx])
		}(i)
	}
	
	wg.Wait()
	
	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to decompress item %d: %w", i, err)
		}
	}
	
	return results, nil
}

// CompressString compresses a string
func CompressString(s string) ([]byte, error) {
	return Compress(nil, []byte(s)), nil
}

// DecompressString decompresses to a string
func DecompressString(data []byte) (string, error) {
	result, err := Decompress(nil, data)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// StreamCompressFile compresses a file using streaming
func StreamCompressFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()
	
	// Create destination directory if needed
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()
	
	return StreamCompress(dst, src)
}

// StreamDecompressFile decompresses a file using streaming
func StreamDecompressFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()
	
	// Create destination directory if needed
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()
	
	return StreamDecompress(dst, src)
}

// CompressHTTPResponse is a helper for HTTP middleware
func CompressHTTPResponse(data []byte, acceptEncoding string) ([]byte, string, error) {
	// Check if client accepts zstd
	if strings.Contains(acceptEncoding, "zstd") {
		compressed := Compress(nil, data)
		return compressed, "zstd", nil
	}
	
	// Return uncompressed if zstd not accepted
	return data, "", nil
}

// EstimateCompressionRatio estimates compression ratio without full compression
func EstimateCompressionRatio(data []byte) float64 {
	if len(data) < 1024 {
		return 1.0 // Too small to estimate
	}
	
	// Sample first 10KB
	sampleSize := 10240
	if len(data) < sampleSize {
		sampleSize = len(data)
	}
	
	sample := data[:sampleSize]
	compressed := Compress(nil, sample)
	
	return float64(len(sample)) / float64(len(compressed))
}

// CompressIfBeneficial only compresses if it reduces size significantly
func CompressIfBeneficial(data []byte, threshold float64) ([]byte, bool, error) {
	if threshold <= 0 {
		threshold = 0.9 // Default: compress if result is <90% of original
	}
	
	compressed := Compress(nil, data)
	
	if float64(len(compressed)) < float64(len(data))*threshold {
		return compressed, true, nil
	}
	
	return data, false, nil
}

// CompressionStats tracks compression statistics
type CompressionStats struct {
	mu                sync.RWMutex
	TotalBytes        int64
	CompressedBytes   int64
	CompressionTime   time.Duration
	DecompressionTime time.Duration
	CompressionCount  int64
	DecompressionCount int64
}

// GlobalStats tracks global compression statistics
var GlobalStats = &CompressionStats{}

// RecordCompression records compression statistics
func (cs *CompressionStats) RecordCompression(originalSize, compressedSize int, duration time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	
	cs.TotalBytes += int64(originalSize)
	cs.CompressedBytes += int64(compressedSize)
	cs.CompressionTime += duration
	cs.CompressionCount++
}

// RecordDecompression records decompression statistics
func (cs *CompressionStats) RecordDecompression(duration time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	
	cs.DecompressionTime += duration
	cs.DecompressionCount++
}

// GetStats returns current statistics
func (cs *CompressionStats) GetStats() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	
	stats := make(map[string]interface{})
	stats["total_bytes"] = cs.TotalBytes
	stats["compressed_bytes"] = cs.CompressedBytes
	
	if cs.TotalBytes > 0 {
		ratio := float64(cs.TotalBytes-cs.CompressedBytes) / float64(cs.TotalBytes) * 100
		stats["compression_ratio"] = fmt.Sprintf("%.2f%%", ratio)
	}
	
	if cs.CompressionCount > 0 {
		avgTime := cs.CompressionTime / time.Duration(cs.CompressionCount)
		stats["avg_compression_time"] = avgTime.String()
	}
	
	if cs.DecompressionCount > 0 {
		avgTime := cs.DecompressionTime / time.Duration(cs.DecompressionCount)
		stats["avg_decompression_time"] = avgTime.String()
	}
	
	stats["compression_count"] = cs.CompressionCount
	stats["decompression_count"] = cs.DecompressionCount
	
	return stats
}
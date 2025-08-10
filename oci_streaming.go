package gozstd

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
)

// OCIStreamCompressor handles streaming compression for large OCI layers
type OCIStreamCompressor struct {
	Level           int
	Workers         int
	ChunkSize       int // Size of chunks for parallel processing
	BufferSize      int // Size of internal buffers
	ProgressFunc    ProgressFunc
	
	// Statistics
	totalBytes      int64
	processedBytes  int64
	compressedBytes int64
}

// DefaultOCIStreamCompressor returns a stream compressor with OCI-optimized defaults
func DefaultOCIStreamCompressor() *OCIStreamCompressor {
	return &OCIStreamCompressor{
		Level:      12,              // Default level 12 for large objects
		Workers:    0,               // Auto-detect
		ChunkSize:  4 * 1024 * 1024, // 4MB chunks
		BufferSize: 128 * 1024,      // 128KB buffers
	}
}

// StreamCompressLarge handles very large OCI layers efficiently
func (sc *OCIStreamCompressor) StreamCompressLarge(dst io.Writer, src io.Reader, estimatedSize int64) error {
	// Auto-configure workers if not set
	if sc.Workers == 0 {
		sc.Workers = sc.autoDetectWorkers(estimatedSize)
	}
	
	// For single-threaded, use simple streaming
	if sc.Workers <= 1 {
		return sc.streamCompressSimple(dst, src)
	}
	
	// For multi-threaded, use parallel chunk processing
	return sc.streamCompressParallel(dst, src)
}

// autoDetectWorkers determines optimal worker count
func (sc *OCIStreamCompressor) autoDetectWorkers(estimatedSize int64) int {
	cpuCount := runtime.NumCPU()
	
	// Don't parallelize small files
	if estimatedSize < 10*1024*1024 { // < 10MB
		return 1
	}
	
	// Scale with file size and CPU
	if estimatedSize < 100*1024*1024 { // < 100MB
		if cpuCount >= 4 {
			return 2
		}
		return 1
	}
	
	if estimatedSize < 1024*1024*1024 { // < 1GB
		if cpuCount >= 8 {
			return 4
		} else if cpuCount >= 4 {
			return 2
		}
		return 1
	}
	
	// Very large files (>= 1GB)
	if cpuCount >= 16 {
		return 8
	} else if cpuCount >= 8 {
		return 4
	} else if cpuCount >= 4 {
		return 2
	}
	return 1
}

// streamCompressSimple handles single-threaded streaming
func (sc *OCIStreamCompressor) streamCompressSimple(dst io.Writer, src io.Reader) error {
	w := NewWriterLevel(dst, sc.Level)
	defer w.Release()
	
	// Use custom buffer size for better performance
	buf := make([]byte, sc.BufferSize)
	var written int64
	
	for {
		n, err := src.Read(buf)
		if n > 0 {
			nw, werr := w.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if nw != n {
				return io.ErrShortWrite
			}
			written += int64(nw)
			atomic.AddInt64(&sc.processedBytes, int64(n))
			
			// Report progress
			if sc.ProgressFunc != nil {
				sc.ProgressFunc(atomic.LoadInt64(&sc.processedBytes), sc.totalBytes)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	
	return w.Close()
}

// streamCompressParallel handles multi-threaded chunk processing
func (sc *OCIStreamCompressor) streamCompressParallel(dst io.Writer, src io.Reader) error {
	// Create a pipeline for parallel compression
	type chunk struct {
		id         int
		data       []byte
		compressed []byte
		err        error
	}
	
	// Channels for the pipeline
	chunkChan := make(chan *chunk, sc.Workers*2)
	compressedChan := make(chan *chunk, sc.Workers*2)
	
	// WaitGroup for workers
	var wg sync.WaitGroup
	
	// Start compression workers
	for i := 0; i < sc.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := NewCCtx()
			defer ctx.Release()
			ctx.SetParameter(ZSTD_c_compressionLevel, sc.Level)
			
			for ch := range chunkChan {
				if ch.err != nil {
					compressedChan <- ch
					continue
				}
				
				compressed, err := ctx.Compress(nil, ch.data)
				if err != nil {
					ch.err = err
				} else {
					ch.compressed = compressed
					atomic.AddInt64(&sc.compressedBytes, int64(len(compressed)))
				}
				
				// Return buffer to pool
				PutBuffer(ch.data)
				ch.data = nil
				
				compressedChan <- ch
			}
		}()
	}
	
	// Start writer goroutine
	writerDone := make(chan error, 1)
	go func() {
		// Buffer for ordering chunks
		pending := make(map[int]*chunk)
		nextID := 0
		
		for ch := range compressedChan {
			if ch.err != nil {
				writerDone <- ch.err
				return
			}
			
			// Store chunk
			pending[ch.id] = ch
			
			// Write sequential chunks
			for {
				if next, ok := pending[nextID]; ok {
					_, err := dst.Write(next.compressed)
					if err != nil {
						writerDone <- err
						return
					}
					delete(pending, nextID)
					nextID++
				} else {
					break
				}
			}
		}
		
		// Check if all chunks were written
		if len(pending) > 0 {
			writerDone <- fmt.Errorf("missing chunks in output")
			return
		}
		
		writerDone <- nil
	}()
	
	// Read and send chunks
	chunkID := 0
	for {
		// Get buffer from pool
		buf := GetBuffer(sc.ChunkSize)
		n, err := io.ReadFull(src, buf[:cap(buf)])
		
		if n > 0 {
			chunkChan <- &chunk{
				id:   chunkID,
				data: buf[:n],
			}
			chunkID++
			atomic.AddInt64(&sc.processedBytes, int64(n))
			
			// Report progress
			if sc.ProgressFunc != nil {
				sc.ProgressFunc(atomic.LoadInt64(&sc.processedBytes), sc.totalBytes)
			}
		}
		
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			// Send error through pipeline
			chunkChan <- &chunk{err: err}
			break
		}
	}
	
	// Close chunk channel and wait for workers
	close(chunkChan)
	wg.Wait()
	close(compressedChan)
	
	// Wait for writer to finish
	return <-writerDone
}

// ChunkedOCICompressor provides chunked compression for very large layers
type ChunkedOCICompressor struct {
	ChunkSize    int  // Size of each chunk (default 4MB)
	Level        int  // Compression level
	MaxChunks    int  // Maximum chunks in memory
	EnableIndex  bool // Create chunk index for random access
}

// DefaultChunkedOCICompressor returns defaults for chunked compression
func DefaultChunkedOCICompressor() *ChunkedOCICompressor {
	return &ChunkedOCICompressor{
		ChunkSize: 4 * 1024 * 1024, // 4MB chunks
		Level:     12,               // Level 12 for large objects
		MaxChunks: 8,                // Max 8 chunks in memory (32MB)
		EnableIndex: false,          // No index by default for OCI compatibility
	}
}

// CompressChunked compresses data in chunks (useful for stargz)
func (cc *ChunkedOCICompressor) CompressChunked(dst io.Writer, src io.Reader) error {
	buf := make([]byte, cc.ChunkSize)
	ctx := NewCCtx()
	defer ctx.Release()
	ctx.SetParameter(ZSTD_c_compressionLevel, cc.Level)
	
	for {
		n, err := io.ReadFull(src, buf)
		if n > 0 {
			compressed, cerr := ctx.Compress(nil, buf[:n])
			if cerr != nil {
				return cerr
			}
			
			// Write compressed chunk
			if _, werr := dst.Write(compressed); werr != nil {
				return werr
			}
			
			// Reset context for next chunk
			ctx.Reset(ZSTD_reset_session_only)
		}
		
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	
	return nil
}

// Helper functions for easy streaming

// StreamCompressOCILayerLarge compresses large layers with optimizations
func StreamCompressOCILayerLarge(dst io.Writer, src io.Reader, estimatedSize int64) error {
	sc := DefaultOCIStreamCompressor()
	return sc.StreamCompressLarge(dst, src, estimatedSize)
}

// StreamCompressOCILayerWithProgress adds progress tracking
func StreamCompressOCILayerWithProgress(dst io.Writer, src io.Reader, progress ProgressFunc) error {
	sc := DefaultOCIStreamCompressor()
	sc.ProgressFunc = progress
	return sc.StreamCompressLarge(dst, src, 0)
}

// StreamCompressOCILayerCustom allows full customization
func StreamCompressOCILayerCustom(dst io.Writer, src io.Reader, level, workers int) error {
	sc := &OCIStreamCompressor{
		Level:      level,
		Workers:    workers,
		ChunkSize:  4 * 1024 * 1024,
		BufferSize: 128 * 1024,
	}
	return sc.StreamCompressLarge(dst, src, 0)
}
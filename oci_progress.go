package gozstd

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// OCIProgressTracker tracks compression progress for OCI layers
type OCIProgressTracker struct {
	mu              sync.RWMutex
	startTime       time.Time
	totalBytes      int64
	processedBytes  int64
	compressedBytes int64
	layerName       string
	callback        OCIProgressCallback
	updateInterval  time.Duration
	lastUpdate      time.Time
}

// OCIProgressCallback is called with progress updates
type OCIProgressCallback func(info *OCIProgressInfo)

// OCIProgressInfo contains progress information
type OCIProgressInfo struct {
	LayerName       string        // Name/ID of the layer being compressed
	TotalBytes      int64         // Total bytes to process (0 if unknown)
	ProcessedBytes  int64         // Bytes processed so far
	CompressedBytes int64         // Bytes after compression
	Percentage      float64       // Percentage complete (0-100)
	Speed           float64       // Processing speed in MB/s
	CompressionRatio float64      // Compression ratio achieved so far
	TimeElapsed     time.Duration // Time elapsed since start
	TimeRemaining   time.Duration // Estimated time remaining
	IsComplete      bool          // Whether compression is complete
}

// NewOCIProgressTracker creates a new progress tracker
func NewOCIProgressTracker(layerName string, totalBytes int64, callback OCIProgressCallback) *OCIProgressTracker {
	return &OCIProgressTracker{
		layerName:      layerName,
		totalBytes:     totalBytes,
		callback:       callback,
		updateInterval: 100 * time.Millisecond, // Update every 100ms
		startTime:      time.Now(),
	}
}

// Start initializes the progress tracker
func (pt *OCIProgressTracker) Start() {
	pt.mu.Lock()
	pt.startTime = time.Now()
	pt.processedBytes = 0
	pt.compressedBytes = 0
	pt.lastUpdate = time.Now()
	pt.mu.Unlock()
	
	// Send initial progress
	if pt.callback != nil {
		pt.callback(pt.getProgressInfo(false))
	}
}

// Update updates the progress
func (pt *OCIProgressTracker) Update(processed, compressed int64) {
	atomic.StoreInt64(&pt.processedBytes, processed)
	atomic.StoreInt64(&pt.compressedBytes, compressed)
	
	// Check if we should send an update
	pt.mu.RLock()
	shouldUpdate := time.Since(pt.lastUpdate) >= pt.updateInterval
	pt.mu.RUnlock()
	
	if shouldUpdate && pt.callback != nil {
		pt.mu.Lock()
		pt.lastUpdate = time.Now()
		pt.mu.Unlock()
		
		pt.callback(pt.getProgressInfo(false))
	}
}

// Complete marks the compression as complete
func (pt *OCIProgressTracker) Complete() {
	if pt.callback != nil {
		pt.callback(pt.getProgressInfo(true))
	}
}

// getProgressInfo builds the progress info structure
func (pt *OCIProgressTracker) getProgressInfo(complete bool) *OCIProgressInfo {
	processed := atomic.LoadInt64(&pt.processedBytes)
	compressed := atomic.LoadInt64(&pt.compressedBytes)
	total := atomic.LoadInt64(&pt.totalBytes)
	
	elapsed := time.Since(pt.startTime)
	
	info := &OCIProgressInfo{
		LayerName:       pt.layerName,
		TotalBytes:      total,
		ProcessedBytes:  processed,
		CompressedBytes: compressed,
		TimeElapsed:     elapsed,
		IsComplete:      complete,
	}
	
	// Calculate percentage
	if total > 0 {
		info.Percentage = float64(processed) / float64(total) * 100
	}
	
	// Calculate speed (MB/s)
	if elapsed.Seconds() > 0 {
		info.Speed = float64(processed) / (1024 * 1024) / elapsed.Seconds()
	}
	
	// Calculate compression ratio
	if compressed > 0 && processed > 0 {
		info.CompressionRatio = float64(processed) / float64(compressed)
	}
	
	// Estimate time remaining
	if total > 0 && processed > 0 && processed < total && info.Speed > 0 {
		remainingBytes := total - processed
		remainingSeconds := float64(remainingBytes) / (info.Speed * 1024 * 1024)
		info.TimeRemaining = time.Duration(remainingSeconds * float64(time.Second))
	}
	
	return info
}

// ProgressWriter wraps an io.Writer with progress tracking
type ProgressWriter struct {
	writer   io.Writer
	tracker  *OCIProgressTracker
	written  int64
}

// NewProgressWriter creates a writer that tracks progress
func NewProgressWriter(w io.Writer, tracker *OCIProgressTracker) *ProgressWriter {
	return &ProgressWriter{
		writer:  w,
		tracker: tracker,
	}
}

// Write implements io.Writer with progress tracking
func (pw *ProgressWriter) Write(p []byte) (n int, err error) {
	n, err = pw.writer.Write(p)
	if n > 0 {
		atomic.AddInt64(&pw.written, int64(n))
		if pw.tracker != nil {
			pw.tracker.Update(atomic.LoadInt64(&pw.written), atomic.LoadInt64(&pw.written))
		}
	}
	return n, err
}

// ProgressReader wraps an io.Reader with progress tracking
type ProgressReader struct {
	reader  io.Reader
	tracker *OCIProgressTracker
	read    int64
}

// NewProgressReader creates a reader that tracks progress
func NewProgressReader(r io.Reader, tracker *OCIProgressTracker) *ProgressReader {
	return &ProgressReader{
		reader:  r,
		tracker: tracker,
	}
}

// Read implements io.Reader with progress tracking
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&pr.read, int64(n))
		if pr.tracker != nil {
			pr.tracker.Update(atomic.LoadInt64(&pr.read), 0)
		}
	}
	return n, err
}

// CompressOCILayerWithProgress compresses with detailed progress tracking
func CompressOCILayerWithProgress(dst, src []byte, layerName string, callback OCIProgressCallback) []byte {
	tracker := NewOCIProgressTracker(layerName, int64(len(src)), callback)
	tracker.Start()
	
	oc := DefaultOCICompressor()
	oc.ProgressCallback = func(processed, total int64) {
		tracker.Update(processed, int64(len(dst)))
	}
	
	result := oc.Compress(dst, src)
	tracker.Complete()
	
	return result
}

// FormatProgressBar creates a text progress bar
func FormatProgressBar(info *OCIProgressInfo) string {
	const barWidth = 40
	
	if info.TotalBytes == 0 {
		// Unknown total, show spinner
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		idx := int(info.TimeElapsed.Seconds()) % len(spinner)
		return fmt.Sprintf("%s %s: %s @ %.2f MB/s",
			spinner[idx],
			info.LayerName,
			formatBytes(info.ProcessedBytes),
			info.Speed)
	}
	
	// Known total, show progress bar
	filled := int(info.Percentage / 100 * barWidth)
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	
	status := fmt.Sprintf("[%s] %.1f%% %s/%s",
		bar,
		info.Percentage,
		formatBytes(info.ProcessedBytes),
		formatBytes(info.TotalBytes))
	
	if info.Speed > 0 {
		status += fmt.Sprintf(" @ %.2f MB/s", info.Speed)
	}
	
	if info.TimeRemaining > 0 {
		status += fmt.Sprintf(" ETA: %s", formatDuration(info.TimeRemaining))
	}
	
	if info.CompressionRatio > 0 {
		status += fmt.Sprintf(" (%.2fx)", info.CompressionRatio)
	}
	
	return status
}

// formatBytes formats bytes in human-readable form
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatDuration formats duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	
	seconds := int(d.Seconds())
	minutes := seconds / 60
	hours := minutes / 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes%60)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds%60)
	}
	return fmt.Sprintf("%ds", seconds)
}

// BatchProgressTracker tracks progress for multiple layers
type BatchProgressTracker struct {
	mu         sync.RWMutex
	layers     map[string]*OCIProgressInfo
	callback   BatchProgressCallback
	totalBytes int64
	doneBytes  int64
}

// BatchProgressCallback receives updates for batch operations
type BatchProgressCallback func(layers map[string]*OCIProgressInfo, overall *OCIProgressInfo)

// NewBatchProgressTracker creates a tracker for multiple layers
func NewBatchProgressTracker(callback BatchProgressCallback) *BatchProgressTracker {
	return &BatchProgressTracker{
		layers:   make(map[string]*OCIProgressInfo),
		callback: callback,
	}
}

// AddLayer adds a layer to track
func (bt *BatchProgressTracker) AddLayer(name string, size int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	
	bt.layers[name] = &OCIProgressInfo{
		LayerName:  name,
		TotalBytes: size,
	}
	bt.totalBytes += size
}

// UpdateLayer updates progress for a specific layer
func (bt *BatchProgressTracker) UpdateLayer(name string, info *OCIProgressInfo) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	
	if oldInfo, exists := bt.layers[name]; exists {
		// Update done bytes
		if info.IsComplete && !oldInfo.IsComplete {
			bt.doneBytes += info.TotalBytes
		}
		bt.layers[name] = info
	}
	
	// Calculate overall progress
	overall := &OCIProgressInfo{
		LayerName:      "Overall",
		TotalBytes:     bt.totalBytes,
		ProcessedBytes: bt.doneBytes,
	}
	
	if bt.totalBytes > 0 {
		overall.Percentage = float64(bt.doneBytes) / float64(bt.totalBytes) * 100
	}
	
	if bt.callback != nil {
		// Make a copy for callback
		layersCopy := make(map[string]*OCIProgressInfo)
		for k, v := range bt.layers {
			layersCopy[k] = v
		}
		bt.callback(layersCopy, overall)
	}
}
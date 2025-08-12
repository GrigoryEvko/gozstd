package gozstd

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks compression/decompression metrics
type Metrics struct {
	// Compression metrics
	CompressionCount        int64
	CompressionBytes        int64
	CompressedBytes         int64
	CompressionTime         int64 // nanoseconds
	CompressionErrors       int64
	
	// Decompression metrics  
	DecompressionCount      int64
	DecompressionBytes      int64
	DecompressedBytes       int64
	DecompressionTime       int64 // nanoseconds
	DecompressionErrors     int64
	
	// Resource metrics
	ActiveContexts          int64
	ContextsCreated         int64
	ContextsReleased        int64
	DictionariesCreated     int64
	DictionariesReleased    int64
	
	// Memory metrics
	CurrentMemoryUsage      int64
	PeakMemoryUsage         int64
	TotalAllocations        int64
	TotalDeallocations      int64
	
	// Performance metrics
	FastestCompression      int64 // nanoseconds
	SlowestCompression      int64 // nanoseconds
	FastestDecompression    int64 // nanoseconds
	SlowestDecompression    int64 // nanoseconds
	
	// Error tracking
	LastError               string
	LastErrorTime           time.Time
	
	mu sync.RWMutex
}

// GlobalMetrics is the global metrics instance
var GlobalMetrics = &Metrics{
	FastestCompression:   int64(^uint64(0) >> 1), // Max int64
	FastestDecompression: int64(^uint64(0) >> 1), // Max int64
}

// RecordCompression records compression metrics
func (m *Metrics) RecordCompression(originalSize, compressedSize int, duration time.Duration, err error) {
	ns := duration.Nanoseconds()
	
	atomic.AddInt64(&m.CompressionCount, 1)
	atomic.AddInt64(&m.CompressionBytes, int64(originalSize))
	atomic.AddInt64(&m.CompressedBytes, int64(compressedSize))
	atomic.AddInt64(&m.CompressionTime, ns)
	
	if err != nil {
		atomic.AddInt64(&m.CompressionErrors, 1)
		m.mu.Lock()
		m.LastError = err.Error()
		m.LastErrorTime = time.Now()
		m.mu.Unlock()
	}
	
	// Update fastest/slowest
	for {
		fastest := atomic.LoadInt64(&m.FastestCompression)
		if ns >= fastest || !atomic.CompareAndSwapInt64(&m.FastestCompression, fastest, ns) {
			break
		}
	}
	
	for {
		slowest := atomic.LoadInt64(&m.SlowestCompression)
		if ns <= slowest || !atomic.CompareAndSwapInt64(&m.SlowestCompression, slowest, ns) {
			break
		}
	}
}

// RecordDecompression records decompression metrics
func (m *Metrics) RecordDecompression(compressedSize, decompressedSize int, duration time.Duration, err error) {
	ns := duration.Nanoseconds()
	
	atomic.AddInt64(&m.DecompressionCount, 1)
	atomic.AddInt64(&m.DecompressionBytes, int64(compressedSize))
	atomic.AddInt64(&m.DecompressedBytes, int64(decompressedSize))
	atomic.AddInt64(&m.DecompressionTime, ns)
	
	if err != nil {
		atomic.AddInt64(&m.DecompressionErrors, 1)
		m.mu.Lock()
		m.LastError = err.Error()
		m.LastErrorTime = time.Now()
		m.mu.Unlock()
	}
	
	// Update fastest/slowest
	for {
		fastest := atomic.LoadInt64(&m.FastestDecompression)
		if ns >= fastest || !atomic.CompareAndSwapInt64(&m.FastestDecompression, fastest, ns) {
			break
		}
	}
	
	for {
		slowest := atomic.LoadInt64(&m.SlowestDecompression)
		if ns <= slowest || !atomic.CompareAndSwapInt64(&m.SlowestDecompression, slowest, ns) {
			break
		}
	}
}

// RecordMemoryUsage records memory usage
func (m *Metrics) RecordMemoryUsage(bytes int64) {
	current := atomic.AddInt64(&m.CurrentMemoryUsage, bytes)
	
	// Update peak if needed
	for {
		peak := atomic.LoadInt64(&m.PeakMemoryUsage)
		if current <= peak || !atomic.CompareAndSwapInt64(&m.PeakMemoryUsage, peak, current) {
			break
		}
	}
	
	if bytes > 0 {
		atomic.AddInt64(&m.TotalAllocations, bytes)
	} else {
		atomic.AddInt64(&m.TotalDeallocations, -bytes)
	}
}

// GetSnapshot returns a snapshot of current metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	lastError := m.LastError
	lastErrorTime := m.LastErrorTime
	m.mu.RUnlock()
	
	return MetricsSnapshot{
		CompressionCount:     atomic.LoadInt64(&m.CompressionCount),
		CompressionBytes:     atomic.LoadInt64(&m.CompressionBytes),
		CompressedBytes:      atomic.LoadInt64(&m.CompressedBytes),
		CompressionTime:      time.Duration(atomic.LoadInt64(&m.CompressionTime)),
		CompressionErrors:    atomic.LoadInt64(&m.CompressionErrors),
		
		DecompressionCount:   atomic.LoadInt64(&m.DecompressionCount),
		DecompressionBytes:   atomic.LoadInt64(&m.DecompressionBytes),
		DecompressedBytes:    atomic.LoadInt64(&m.DecompressedBytes),
		DecompressionTime:    time.Duration(atomic.LoadInt64(&m.DecompressionTime)),
		DecompressionErrors:  atomic.LoadInt64(&m.DecompressionErrors),
		
		ActiveContexts:       atomic.LoadInt64(&m.ActiveContexts),
		ContextsCreated:      atomic.LoadInt64(&m.ContextsCreated),
		ContextsReleased:     atomic.LoadInt64(&m.ContextsReleased),
		DictionariesCreated:  atomic.LoadInt64(&m.DictionariesCreated),
		DictionariesReleased: atomic.LoadInt64(&m.DictionariesReleased),
		
		CurrentMemoryUsage:   atomic.LoadInt64(&m.CurrentMemoryUsage),
		PeakMemoryUsage:      atomic.LoadInt64(&m.PeakMemoryUsage),
		TotalAllocations:     atomic.LoadInt64(&m.TotalAllocations),
		TotalDeallocations:   atomic.LoadInt64(&m.TotalDeallocations),
		
		FastestCompression:   time.Duration(atomic.LoadInt64(&m.FastestCompression)),
		SlowestCompression:   time.Duration(atomic.LoadInt64(&m.SlowestCompression)),
		FastestDecompression: time.Duration(atomic.LoadInt64(&m.FastestDecompression)),
		SlowestDecompression: time.Duration(atomic.LoadInt64(&m.SlowestDecompression)),
		
		LastError:     lastError,
		LastErrorTime: lastErrorTime,
		Timestamp:     time.Now(),
	}
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.CompressionCount, 0)
	atomic.StoreInt64(&m.CompressionBytes, 0)
	atomic.StoreInt64(&m.CompressedBytes, 0)
	atomic.StoreInt64(&m.CompressionTime, 0)
	atomic.StoreInt64(&m.CompressionErrors, 0)
	
	atomic.StoreInt64(&m.DecompressionCount, 0)
	atomic.StoreInt64(&m.DecompressionBytes, 0)
	atomic.StoreInt64(&m.DecompressedBytes, 0)
	atomic.StoreInt64(&m.DecompressionTime, 0)
	atomic.StoreInt64(&m.DecompressionErrors, 0)
	
	atomic.StoreInt64(&m.ActiveContexts, 0)
	atomic.StoreInt64(&m.ContextsCreated, 0)
	atomic.StoreInt64(&m.ContextsReleased, 0)
	atomic.StoreInt64(&m.DictionariesCreated, 0)
	atomic.StoreInt64(&m.DictionariesReleased, 0)
	
	atomic.StoreInt64(&m.CurrentMemoryUsage, 0)
	atomic.StoreInt64(&m.PeakMemoryUsage, 0)
	atomic.StoreInt64(&m.TotalAllocations, 0)
	atomic.StoreInt64(&m.TotalDeallocations, 0)
	
	atomic.StoreInt64(&m.FastestCompression, int64(^uint64(0)>>1))
	atomic.StoreInt64(&m.SlowestCompression, 0)
	atomic.StoreInt64(&m.FastestDecompression, int64(^uint64(0)>>1))
	atomic.StoreInt64(&m.SlowestDecompression, 0)
	
	m.mu.Lock()
	m.LastError = ""
	m.LastErrorTime = time.Time{}
	m.mu.Unlock()
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	// Compression metrics
	CompressionCount    int64
	CompressionBytes    int64
	CompressedBytes     int64
	CompressionTime     time.Duration
	CompressionErrors   int64
	
	// Decompression metrics
	DecompressionCount  int64
	DecompressionBytes  int64
	DecompressedBytes   int64
	DecompressionTime   time.Duration
	DecompressionErrors int64
	
	// Resource metrics
	ActiveContexts       int64
	ContextsCreated      int64
	ContextsReleased     int64
	DictionariesCreated  int64
	DictionariesReleased int64
	
	// Memory metrics
	CurrentMemoryUsage int64
	PeakMemoryUsage    int64
	TotalAllocations   int64
	TotalDeallocations int64
	
	// Performance metrics
	FastestCompression   time.Duration
	SlowestCompression   time.Duration
	FastestDecompression time.Duration
	SlowestDecompression time.Duration
	
	// Error tracking
	LastError     string
	LastErrorTime time.Time
	
	// Snapshot metadata
	Timestamp time.Time
}

// CompressionRatio returns the average compression ratio
func (ms *MetricsSnapshot) CompressionRatio() float64 {
	if ms.CompressedBytes == 0 {
		return 0
	}
	return float64(ms.CompressionBytes) / float64(ms.CompressedBytes)
}

// AverageCompressionSpeed returns average compression speed in MB/s
func (ms *MetricsSnapshot) AverageCompressionSpeed() float64 {
	if ms.CompressionTime == 0 {
		return 0
	}
	mbytes := float64(ms.CompressionBytes) / (1024 * 1024)
	seconds := ms.CompressionTime.Seconds()
	return mbytes / seconds
}

// AverageDecompressionSpeed returns average decompression speed in MB/s
func (ms *MetricsSnapshot) AverageDecompressionSpeed() float64 {
	if ms.DecompressionTime == 0 {
		return 0
	}
	mbytes := float64(ms.DecompressedBytes) / (1024 * 1024)
	seconds := ms.DecompressionTime.Seconds()
	return mbytes / seconds
}

// String returns a human-readable representation
func (ms *MetricsSnapshot) String() string {
	return fmt.Sprintf(`Compression Metrics:
  Count: %d, Errors: %d
  Total: %d bytes -> %d bytes (ratio: %.2fx)
  Speed: %.2f MB/s
  Time: fastest=%v, slowest=%v, avg=%v

Decompression Metrics:
  Count: %d, Errors: %d  
  Total: %d bytes -> %d bytes
  Speed: %.2f MB/s
  Time: fastest=%v, slowest=%v, avg=%v

Memory Usage:
  Current: %d bytes, Peak: %d bytes
  Allocated: %d bytes, Deallocated: %d bytes

Resources:
  Active Contexts: %d
  Created: %d contexts, %d dictionaries
  Released: %d contexts, %d dictionaries`,
		ms.CompressionCount, ms.CompressionErrors,
		ms.CompressionBytes, ms.CompressedBytes, ms.CompressionRatio(),
		ms.AverageCompressionSpeed(),
		ms.FastestCompression, ms.SlowestCompression,
		ms.CompressionTime/time.Duration(ms.CompressionCount+1),
		
		ms.DecompressionCount, ms.DecompressionErrors,
		ms.DecompressionBytes, ms.DecompressedBytes,
		ms.AverageDecompressionSpeed(),
		ms.FastestDecompression, ms.SlowestDecompression,
		ms.DecompressionTime/time.Duration(ms.DecompressionCount+1),
		
		ms.CurrentMemoryUsage, ms.PeakMemoryUsage,
		ms.TotalAllocations, ms.TotalDeallocations,
		
		ms.ActiveContexts,
		ms.ContextsCreated, ms.DictionariesCreated,
		ms.ContextsReleased, ms.DictionariesReleased,
	)
}

// JSON returns metrics as JSON
func (ms *MetricsSnapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(ms, "", "  ")
}

// Monitor provides continuous monitoring
type Monitor struct {
	interval time.Duration
	stopCh   chan struct{}
	callbacks []func(*MetricsSnapshot)
	mu       sync.RWMutex
}

// NewMonitor creates a new monitor
func NewMonitor(interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	
	return &Monitor{
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// AddCallback adds a monitoring callback
func (m *Monitor) AddCallback(cb func(*MetricsSnapshot)) {
	m.mu.Lock()
	m.callbacks = append(m.callbacks, cb)
	m.mu.Unlock()
}

// Start starts monitoring
func (m *Monitor) Start() {
	go m.run()
}

// Stop stops monitoring
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// run is the monitoring loop
func (m *Monitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			snapshot := GlobalMetrics.GetSnapshot()
			
			m.mu.RLock()
			callbacks := m.callbacks
			m.mu.RUnlock()
			
			for _, cb := range callbacks {
				cb(&snapshot)
			}
			
		case <-m.stopCh:
			return
		}
	}
}

// HealthCheck performs a health check
type HealthCheck struct {
	MaxErrorRate      float64       // Maximum error rate (0-1)
	MinCompressionRate float64      // Minimum compression operations per second
	MaxMemoryUsage    int64         // Maximum memory usage in bytes
	MaxResponseTime   time.Duration // Maximum average response time
}

// DefaultHealthCheck provides default health check thresholds
var DefaultHealthCheck = &HealthCheck{
	MaxErrorRate:       0.01, // 1% error rate
	MinCompressionRate: 0,    // No minimum rate
	MaxMemoryUsage:     1 << 30, // 1GB
	MaxResponseTime:    time.Second,
}

// Check performs a health check
func (hc *HealthCheck) Check() error {
	snapshot := GlobalMetrics.GetSnapshot()
	
	// Check error rate
	totalOps := snapshot.CompressionCount + snapshot.DecompressionCount
	totalErrors := snapshot.CompressionErrors + snapshot.DecompressionErrors
	
	if totalOps > 0 {
		errorRate := float64(totalErrors) / float64(totalOps)
		if errorRate > hc.MaxErrorRate {
			return fmt.Errorf("error rate too high: %.2f%% (max: %.2f%%)",
				errorRate*100, hc.MaxErrorRate*100)
		}
	}
	
	// Check memory usage
	if snapshot.CurrentMemoryUsage > hc.MaxMemoryUsage {
		return fmt.Errorf("memory usage too high: %d bytes (max: %d bytes)",
			snapshot.CurrentMemoryUsage, hc.MaxMemoryUsage)
	}
	
	// Check response time
	if snapshot.CompressionCount > 0 {
		avgTime := snapshot.CompressionTime / time.Duration(snapshot.CompressionCount)
		if avgTime > hc.MaxResponseTime {
			return fmt.Errorf("compression too slow: %v (max: %v)",
				avgTime, hc.MaxResponseTime)
		}
	}
	
	return nil
}

// RuntimeStats provides runtime statistics
func RuntimeStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	stats := make(map[string]interface{})
	stats["goroutines"] = runtime.NumGoroutine()
	stats["memory_alloc"] = m.Alloc
	stats["memory_total_alloc"] = m.TotalAlloc
	stats["memory_sys"] = m.Sys
	stats["memory_heap_alloc"] = m.HeapAlloc
	stats["memory_heap_sys"] = m.HeapSys
	stats["gc_count"] = m.NumGC
	stats["gc_pause_total"] = m.PauseTotalNs
	
	return stats
}
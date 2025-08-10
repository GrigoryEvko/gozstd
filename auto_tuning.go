package gozstd

import (
	"runtime"
	"sync"
	"time"
)

// AutoTuner automatically selects optimal compression parameters based on system resources.
type AutoTuner struct {
	mu              sync.RWMutex
	cpuCount        int
	memoryAvailable uint64
	lastUpdate      time.Time
	
	// Cached optimal settings
	optimalLevel    int
	optimalWorkers  int
	optimalWindowLog int
}

var (
	globalAutoTuner *AutoTuner
	autoTunerOnce   sync.Once
)

// GetAutoTuner returns the global auto-tuner instance.
func GetAutoTuner() *AutoTuner {
	autoTunerOnce.Do(func() {
		globalAutoTuner = &AutoTuner{
			cpuCount:       runtime.NumCPU(),
			lastUpdate:     time.Now(),
			optimalLevel:   DefaultCompressionLevel,
			optimalWorkers: 0, // 0 means single-threaded
		}
		globalAutoTuner.updateSystemInfo()
	})
	return globalAutoTuner
}

// updateSystemInfo updates system resource information.
func (at *AutoTuner) updateSystemInfo() {
	at.mu.Lock()
	defer at.mu.Unlock()
	
	// Update CPU count (might change in containers)
	at.cpuCount = runtime.NumCPU()
	
	// Get memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Available memory is roughly: system memory - allocated - reserved
	// This is a simplified estimation
	at.memoryAvailable = m.Sys - m.Alloc
	
	at.lastUpdate = time.Now()
}

// GetOptimalCompressionLevel returns the optimal compression level based on available resources.
// It balances compression ratio, speed, and resource usage.
func (at *AutoTuner) GetOptimalCompressionLevel() int {
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Refresh if data is stale (older than 1 minute)
	if time.Since(at.lastUpdate) > time.Minute {
		at.mu.RUnlock()
		at.updateSystemInfo()
		at.mu.RLock()
	}
	
	// Auto-select compression level based on available memory
	// Higher levels use more memory but compress better
	if at.memoryAvailable < 100*1024*1024 { // Less than 100MB
		return 1 // Fast compression, low memory
	} else if at.memoryAvailable < 500*1024*1024 { // Less than 500MB
		return 3 // Default compression
	} else if at.memoryAvailable < 1024*1024*1024 { // Less than 1GB
		return 5 // Better compression
	} else {
		// Plenty of memory available
		if at.cpuCount >= 8 {
			return 9 // High compression for powerful systems
		} else if at.cpuCount >= 4 {
			return 7 // Good compression for moderate systems
		} else {
			return 5 // Balanced for low-core systems
		}
	}
}

// GetOptimalWorkers returns the optimal number of worker threads.
func (at *AutoTuner) GetOptimalWorkers(dataSize int) int {
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Don't use workers for small data
	if dataSize < 256*1024 { // Less than 256KB
		return 0 // Single-threaded is faster for small data
	}
	
	// Use workers based on CPU count and data size
	if at.cpuCount <= 2 {
		return 0 // Not worth the overhead on dual-core
	} else if at.cpuCount <= 4 {
		if dataSize > 1024*1024 { // More than 1MB
			return 2
		}
		return 0
	} else if at.cpuCount <= 8 {
		if dataSize > 10*1024*1024 { // More than 10MB
			return 4
		} else if dataSize > 1024*1024 {
			return 2
		}
		return 0
	} else {
		// Many cores available
		if dataSize > 100*1024*1024 { // More than 100MB
			return at.cpuCount / 2 // Use half the cores
		} else if dataSize > 10*1024*1024 {
			return 4
		} else if dataSize > 1024*1024 {
			return 2
		}
		return 0
	}
}

// GetOptimalWindowLog returns the optimal window size based on data characteristics.
func (at *AutoTuner) GetOptimalWindowLog(dataSize int) int {
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Window size affects compression ratio and memory usage
	// Larger windows = better compression but more memory
	
	// Return 0 to use ZSTD defaults for very low memory
	if at.memoryAvailable < 50*1024*1024 { // Less than 50MB
		return 0 // Use default (will be set by ZSTD)
	}
	
	// Calculate based on data size
	if dataSize < 16*1024 { // Less than 16KB
		return 10 // Small window for small data
	} else if dataSize < 256*1024 { // Less than 256KB
		return 14
	} else if dataSize < 1024*1024 { // Less than 1MB
		return 17
	} else if dataSize < 16*1024*1024 { // Less than 16MB
		return 20
	} else {
		// Large data
		if at.memoryAvailable > 1024*1024*1024 { // More than 1GB available
			return 23 // Large window for best compression
		}
		return 21
	}
}

// ShouldEnableChecksum returns whether checksum should be enabled.
func (at *AutoTuner) ShouldEnableChecksum(critical bool) bool {
	// Always enable for critical data
	if critical {
		return true
	}
	
	at.mu.RLock()
	defer at.mu.RUnlock()
	
	// Enable checksum if we have plenty of CPU
	// The overhead is minimal on modern CPUs
	return at.cpuCount >= 4
}

// AutoCompressLevel compresses with automatically selected parameters.
func AutoCompressLevel(dst, src []byte) []byte {
	tuner := GetAutoTuner()
	level := tuner.GetOptimalCompressionLevel()
	return CompressLevel(dst, src, level)
}

// NewCCtxAuto creates a compression context with auto-tuned parameters.
func NewCCtxAuto() *CCtx {
	ctx := NewCCtx()
	tuner := GetAutoTuner()
	
	// Set auto-tuned parameters
	ctx.SetParameter(ZSTD_c_compressionLevel, tuner.GetOptimalCompressionLevel())
	
	// Note: Workers are better set per-compression based on data size
	// Window log is also data-dependent
	
	return ctx
}

// CompressAuto compresses with fully automatic parameter selection.
func CompressAuto(dst, src []byte) []byte {
	tuner := GetAutoTuner()
	
	// Select parameters based on data size
	level := tuner.GetOptimalCompressionLevel()
	workers := tuner.GetOptimalWorkers(len(src))
	
	if workers > 0 {
		// Use advanced API for multi-threaded compression
		ctx := NewCCtx()
		defer ctx.Release()
		
		ctx.SetParameter(ZSTD_c_compressionLevel, level)
		ctx.SetParameter(ZSTD_c_nbWorkers, workers)
		
		windowLog := tuner.GetOptimalWindowLog(len(src))
		if windowLog > 0 {
			ctx.SetParameter(ZSTD_c_windowLog, windowLog)
		}
		
		if tuner.ShouldEnableChecksum(false) {
			ctx.SetParameter(ZSTD_c_checksumFlag, 1)
		}
		
		result, err := ctx.Compress(dst, src)
		if err != nil {
			// Fallback to simple compression
			return CompressLevel(dst, src, level)
		}
		return result
	}
	
	// Single-threaded compression
	return CompressLevel(dst, src, level)
}
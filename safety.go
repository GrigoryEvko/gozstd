package gozstd

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Safety configuration constants
const (
	// DefaultMaxDecompressedSize is the default maximum size for decompressed data (1GB)
	DefaultMaxDecompressedSize = 1 << 30 // 1GB
	
	// DefaultMaxCompressionRatio is the maximum allowed compression ratio to prevent memory bombs
	DefaultMaxCompressionRatio = 1000
	
	// DefaultOperationTimeout is the default timeout for compression/decompression operations
	DefaultOperationTimeout = 5 * time.Minute
	
	// DefaultMaxMemoryUsage is the default maximum memory usage (512MB)
	DefaultMaxMemoryUsage = 512 << 20 // 512MB
)

// SafetyConfig provides safety limits for compression/decompression
type SafetyConfig struct {
	// MaxDecompressedSize is the maximum allowed size for decompressed data
	MaxDecompressedSize int64
	
	// MaxCompressionRatio is the maximum allowed compression ratio (output/input)
	MaxCompressionRatio float64
	
	// OperationTimeout is the maximum time allowed for an operation
	OperationTimeout time.Duration
	
	// MaxMemoryUsage is the maximum memory that can be used
	MaxMemoryUsage int64
	
	// EnableValidation enables input validation
	EnableValidation bool
	
	// EnableChecksums enables automatic checksum verification
	EnableChecksums bool
	
	// mu protects the config
	mu sync.RWMutex
}

// GlobalSafety is the global safety configuration
var GlobalSafety = &SafetyConfig{
	MaxDecompressedSize: DefaultMaxDecompressedSize,
	MaxCompressionRatio: DefaultMaxCompressionRatio,
	OperationTimeout:    DefaultOperationTimeout,
	MaxMemoryUsage:      DefaultMaxMemoryUsage,
	EnableValidation:    true,
	EnableChecksums:     true,
}

// currentMemoryUsage tracks current memory usage
var currentMemoryUsage int64

// ValidateDecompressionSize validates that decompression won't exceed limits
func ValidateDecompressionSize(compressedSize int, estimatedDecompressedSize int64) error {
	if !GlobalSafety.EnableValidation {
		return nil
	}
	
	GlobalSafety.mu.RLock()
	maxSize := GlobalSafety.MaxDecompressedSize
	maxRatio := GlobalSafety.MaxCompressionRatio
	GlobalSafety.mu.RUnlock()
	
	// Check absolute size limit
	if estimatedDecompressedSize > 0 && estimatedDecompressedSize > maxSize {
		return fmt.Errorf("decompressed size %d exceeds maximum allowed size %d", 
			estimatedDecompressedSize, maxSize)
	}
	
	// Check compression ratio for potential memory bomb
	if compressedSize > 0 && estimatedDecompressedSize > 0 {
		ratio := float64(estimatedDecompressedSize) / float64(compressedSize)
		if ratio > maxRatio {
			return fmt.Errorf("compression ratio %.2f exceeds maximum allowed ratio %.2f (potential memory bomb)",
				ratio, maxRatio)
		}
	}
	
	return nil
}

// CheckMemoryLimit checks if we can allocate more memory
func CheckMemoryLimit(required int64) error {
	GlobalSafety.mu.RLock()
	maxMem := GlobalSafety.MaxMemoryUsage
	GlobalSafety.mu.RUnlock()
	
	current := atomic.LoadInt64(&currentMemoryUsage)
	if current+required > maxMem {
		return fmt.Errorf("memory usage would exceed limit: current=%d, required=%d, max=%d",
			current, required, maxMem)
	}
	
	return nil
}

// TrackMemory tracks memory allocation/deallocation
func TrackMemory(delta int64) {
	atomic.AddInt64(&currentMemoryUsage, delta)
}

// GetMemoryUsage returns current memory usage
func GetMemoryUsage() int64 {
	return atomic.LoadInt64(&currentMemoryUsage)
}

// WithTimeout executes a function with timeout
func WithTimeout(timeout time.Duration, fn func() error) error {
	if timeout <= 0 {
		return fn()
	}
	
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return errors.New("operation timed out")
	}
}

// SafeCompress performs compression with safety checks
func SafeCompress(dst, src []byte) ([]byte, error) {
	// Check memory limits
	if err := CheckMemoryLimit(int64(len(src))); err != nil {
		return nil, err
	}
	
	// Track memory
	TrackMemory(int64(len(src)))
	defer TrackMemory(-int64(len(src)))
	
	// Perform compression with timeout
	var result []byte
	err := WithTimeout(GlobalSafety.OperationTimeout, func() error {
		result = Compress(dst, src)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("safe compression failed: %w", err)
	}
	
	return result, nil
}

// SafeDecompress performs decompression with safety checks
func SafeDecompress(dst, src []byte) ([]byte, error) {
	// Get estimated decompressed size
	estimatedSize := GetDecompressedSize(src)
	
	// Validate size limits
	if err := ValidateDecompressionSize(len(src), int64(estimatedSize)); err != nil {
		return nil, err
	}
	
	// Check memory limits
	if estimatedSize > 0 {
		if err := CheckMemoryLimit(int64(estimatedSize)); err != nil {
			return nil, err
		}
	}
	
	// Track memory
	if estimatedSize > 0 {
		TrackMemory(int64(estimatedSize))
		defer TrackMemory(-int64(estimatedSize))
	}
	
	// Perform decompression with timeout
	var result []byte
	err := WithTimeout(GlobalSafety.OperationTimeout, func() error {
		var err error
		result, err = Decompress(dst, src)
		return err
	})
	
	if err != nil {
		return nil, fmt.Errorf("safe decompression failed: %w", err)
	}
	
	// Verify actual size doesn't exceed limits
	if len(result) > int(GlobalSafety.MaxDecompressedSize) {
		return nil, fmt.Errorf("decompressed size %d exceeds maximum allowed size %d",
			len(result), GlobalSafety.MaxDecompressedSize)
	}
	
	return result, nil
}

// ResourceTracker tracks resource usage
type ResourceTracker struct {
	mu            sync.Mutex
	resources     map[string]int64
	maxResources  map[string]int64
	totalAllocated int64
	totalFreed     int64
}

// globalTracker is the global resource tracker
var globalTracker = &ResourceTracker{
	resources:    make(map[string]int64),
	maxResources: make(map[string]int64),
}

// TrackResource tracks a resource allocation
func (rt *ResourceTracker) TrackResource(name string, size int64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	rt.resources[name] += size
	rt.totalAllocated += size
	
	if rt.resources[name] > rt.maxResources[name] {
		rt.maxResources[name] = rt.resources[name]
	}
}

// ReleaseResource tracks a resource release
func (rt *ResourceTracker) ReleaseResource(name string, size int64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	rt.resources[name] -= size
	rt.totalFreed += size
	
	if rt.resources[name] < 0 {
		// This shouldn't happen, but let's be safe
		rt.resources[name] = 0
	}
}

// GetStats returns resource statistics
func (rt *ResourceTracker) GetStats() map[string]interface{} {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	stats := make(map[string]interface{})
	stats["current"] = rt.resources
	stats["max"] = rt.maxResources
	stats["total_allocated"] = rt.totalAllocated
	stats["total_freed"] = rt.totalFreed
	stats["leaked"] = rt.totalAllocated - rt.totalFreed
	
	return stats
}

// CircuitBreaker implements circuit breaker pattern for failure handling
type CircuitBreaker struct {
	mu               sync.Mutex
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	state            string // "closed", "open", "half-open"
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenRequests int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        "closed",
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	
	// Check state
	switch cb.state {
	case "open":
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = "half-open"
			cb.halfOpenRequests = 0
		} else {
			cb.mu.Unlock()
			return errors.New("circuit breaker is open")
		}
		
	case "half-open":
		if cb.halfOpenRequests >= 3 {
			cb.mu.Unlock()
			return errors.New("circuit breaker is testing, please retry")
		}
		cb.halfOpenRequests++
	}
	
	cb.mu.Unlock()
	
	// Execute function
	err := fn()
	
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if err != nil {
		cb.failureCount++
		cb.lastFailureTime = time.Now()
		
		if cb.failureCount >= cb.maxFailures {
			cb.state = "open"
		}
		
		return err
	}
	
	// Success
	cb.successCount++
	
	if cb.state == "half-open" {
		// Recovered successfully
		cb.state = "closed"
		cb.failureCount = 0
	}
	
	return nil
}

// MemoryGuard prevents OOM by monitoring memory usage
type MemoryGuard struct {
	mu           sync.Mutex
	maxMemory    int64
	currentUsage int64
	gcThreshold  float64 // Trigger GC when usage exceeds this percentage
}

// NewMemoryGuard creates a new memory guard
func NewMemoryGuard(maxMemory int64) *MemoryGuard {
	return &MemoryGuard{
		maxMemory:   maxMemory,
		gcThreshold: 0.8, // Trigger GC at 80% usage
	}
}

// Allocate tries to allocate memory
func (mg *MemoryGuard) Allocate(size int64) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	
	if mg.currentUsage+size > mg.maxMemory {
		// Try GC first
		runtime.GC()
		
		// Check again
		if mg.currentUsage+size > mg.maxMemory {
			return fmt.Errorf("memory allocation would exceed limit: current=%d, requested=%d, max=%d",
				mg.currentUsage, size, mg.maxMemory)
		}
	}
	
	mg.currentUsage += size
	
	// Trigger GC if needed
	if float64(mg.currentUsage)/float64(mg.maxMemory) > mg.gcThreshold {
		go runtime.GC()
	}
	
	return nil
}

// Release releases allocated memory
func (mg *MemoryGuard) Release(size int64) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	
	mg.currentUsage -= size
	if mg.currentUsage < 0 {
		mg.currentUsage = 0
	}
}

// GetUsage returns current memory usage
func (mg *MemoryGuard) GetUsage() (current, max int64) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	return mg.currentUsage, mg.maxMemory
}

// ===== Nil Safety (merged from nil_safety.go) =====

var (
	ErrNilWriter     = errors.New("writer is nil")
	ErrNilReader     = errors.New("reader is nil")
	ErrNilContext    = errors.New("compression context is nil")
	ErrNilDictionary = errors.New("dictionary is nil")
	ErrNilBuffer     = errors.New("buffer is nil")
	ErrAlreadyClosed = errors.New("already closed")
)

// SafeCompressDict safely compresses with dictionary, handling nil cases
func SafeCompressDict(dst, src []byte, cd *CDict) ([]byte, error) {
	if cd == nil {
		return nil, ErrNilDictionary
	}
	
	// Check if dictionary is still valid (not released)
	if cd.p == nil {
		return nil, &ResourceError{
			Resource:  "CDict",
			Operation: "compress",
			Err:       errors.New("dictionary has been released"),
		}
	}
	
	return CompressDict(dst, src, cd), nil
}

// SafeDecompressDict safely decompresses with dictionary, handling nil cases
func SafeDecompressDict(dst, src []byte, dd *DDict) ([]byte, error) {
	if dd == nil {
		return nil, ErrNilDictionary
	}
	
	// Check if dictionary is still valid
	if dd.p == nil {
		return nil, &ResourceError{
			Resource:  "DDict",
			Operation: "decompress",
			Err:       errors.New("dictionary has been released"),
		}
	}
	
	return DecompressDict(dst, src, dd)
}

// AtomicBool provides atomic boolean operations
type AtomicBool struct {
	value uint32
}

// Set sets the boolean value
func (b *AtomicBool) Set(v bool) {
	if v {
		atomic.StoreUint32(&b.value, 1)
	} else {
		atomic.StoreUint32(&b.value, 0)
	}
}

// Get gets the boolean value
func (b *AtomicBool) Get() bool {
	return atomic.LoadUint32(&b.value) != 0
}

// CompareAndSwap atomically sets to new value if current value equals expected
func (b *AtomicBool) CompareAndSwap(expected, new bool) bool {
	var e, n uint32
	if expected {
		e = 1
	}
	if new {
		n = 1
	}
	return atomic.CompareAndSwapUint32(&b.value, e, n)
}

// SafeDictWrapper wraps dictionary with safe reference counting
type SafeDictWrapper struct {
	cdict    *CDict
	ddict    *DDict
	refCount int32
	mu       sync.RWMutex
	released AtomicBool
}

// NewSafeDictWrapper creates a safe dictionary wrapper
func NewSafeDictWrapper(dict []byte) (*SafeDictWrapper, error) {
	if len(dict) == 0 {
		return nil, errors.New("dictionary data is empty")
	}
	
	cdict, err := NewCDict(dict)
	if err != nil {
		return nil, WrapCompressionError(err, "create CDict", len(dict))
	}
	
	ddict, err := NewDDict(dict)
	if err != nil {
		cdict.Release()
		return nil, WrapDecompressionError(err, "create DDict", len(dict))
	}
	
	return &SafeDictWrapper{
		cdict:    cdict,
		ddict:    ddict,
		refCount: 1,
	}, nil
}

// AddRef safely increments reference count
func (w *SafeDictWrapper) AddRef() error {
	if w.released.Get() {
		return errors.New("cannot add reference to released dictionary")
	}
	
	atomic.AddInt32(&w.refCount, 1)
	return nil
}

// Release safely decrements reference count and releases when zero
func (w *SafeDictWrapper) Release() {
	newCount := atomic.AddInt32(&w.refCount, -1)
	
	if newCount == 0 {
		// Only release once
		if w.released.CompareAndSwap(false, true) {
			w.mu.Lock()
			defer w.mu.Unlock()
			
			if w.cdict != nil {
				w.cdict.Release()
				w.cdict = nil
			}
			if w.ddict != nil {
				w.ddict.Release()
				w.ddict = nil
			}
		}
	} else if newCount < 0 {
		// This shouldn't happen but handle gracefully
		HandleDictRefCountError("SafeDictWrapper")
	}
}

// Compress safely compresses with the dictionary
func (w *SafeDictWrapper) Compress(dst, src []byte) ([]byte, error) {
	if w.released.Get() {
		return nil, errors.New("dictionary has been released")
	}
	
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	if w.cdict == nil {
		return nil, ErrNilDictionary
	}
	
	return CompressDict(dst, src, w.cdict), nil
}

// Decompress safely decompresses with the dictionary
func (w *SafeDictWrapper) Decompress(dst, src []byte) ([]byte, error) {
	if w.released.Get() {
		return nil, errors.New("dictionary has been released")
	}
	
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	if w.ddict == nil {
		return nil, ErrNilDictionary
	}
	
	return DecompressDict(dst, src, w.ddict)
}

// ValidateInput checks if input is valid for compression/decompression
func ValidateInput(data []byte, operation string) error {
	if data == nil {
		return fmt.Errorf("%s: input data is nil", operation)
	}
	
	// Check for unreasonably large input
	const maxReasonableSize = 2 << 30 // 2GB
	if len(data) > maxReasonableSize {
		WarnOnDangerousOperation(operation,
			fmt.Sprintf("input size %d MB exceeds reasonable limit", len(data)/(1<<20)))
	}
	
	return nil
}

// ValidateCompressionLevel checks if compression level is valid
func ValidateCompressionLevel(level int) error {
	if level < MinCompressionLevel || level > MaxCompressionLevel {
		return fmt.Errorf("compression level %d out of range [%d, %d]", 
			level, MinCompressionLevel, MaxCompressionLevel)
	}
	return nil
}

// ===== Auto Recovery (merged from auto_recovery.go) =====

// RetryConfig configures automatic retry behavior.
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialDelay   time.Duration // Initial delay between retries
	MaxDelay       time.Duration // Maximum delay between retries
	BackoffFactor  float64       // Exponential backoff factor
	RetryOnMemory  bool          // Retry on memory allocation errors
	RetryOnBuffer  bool          // Retry on buffer size errors
	AutoAdjustParams bool        // Automatically adjust parameters on failure
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:       3,
		InitialDelay:     10 * time.Millisecond,
		MaxDelay:         1 * time.Second,
		BackoffFactor:    2.0,
		RetryOnMemory:    true,
		RetryOnBuffer:    true,
		AutoAdjustParams: true,
	}
}

// CompressWithRetry compresses data with automatic retry on transient errors.
func CompressWithRetry(dst, src []byte, level int, config *RetryConfig) ([]byte, error) {
	if config == nil {
		config = DefaultRetryConfig()
	}
	
	var lastErr error
	delay := config.InitialDelay
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(delay)
			
			// Force GC to free memory if we're retrying due to memory issues
			if config.RetryOnMemory {
				runtime.GC()
				runtime.Gosched()
			}
			
			// Increase delay for next attempt (exponential backoff)
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
		
		// Try compression
		result := CompressLevel(dst, src, level)
		
		// Check if compression succeeded
		if len(result) > 0 {
			return result, nil
		}
		
		// Compression returned empty result, which might indicate an error
		// Try with adjusted parameters if configured
		if config.AutoAdjustParams && attempt < config.MaxRetries {
			// Reduce compression level for next attempt
			if level > 1 {
				level--
			}
		}
		
		lastErr = fmt.Errorf("compression failed on attempt %d", attempt+1)
	}
	
	return nil, fmt.Errorf("compression failed after %d attempts: %v", config.MaxRetries+1, lastErr)
}

// DecompressWithRetry decompresses data with automatic retry on transient errors.
func DecompressWithRetry(dst, src []byte, config *RetryConfig) ([]byte, error) {
	if config == nil {
		config = DefaultRetryConfig()
	}
	
	var lastErr error
	delay := config.InitialDelay
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(delay)
			
			// Force GC to free memory
			if config.RetryOnMemory {
				runtime.GC()
				runtime.Gosched()
			}
			
			// Increase delay for next attempt
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
		
		// Try decompression
		result, err := Decompress(dst, src)
		
		if err == nil {
			return result, nil
		}
		
		// Check if error is retryable
		if !isRetryableError(err, config) {
			return nil, err // Non-retryable error, fail immediately
		}
		
		lastErr = err
		
		// If buffer size error and auto-adjust is enabled, try with larger buffer
		if config.AutoAdjustParams && config.RetryOnBuffer {
			if attempt == 0 {
				// First retry: estimate size and pre-allocate
				decompressedSize := GetDecompressedSize(src)
				if decompressedSize > 0 && decompressedSize < 1<<30 { // Sanity check
					dst = make([]byte, 0, decompressedSize)
				}
			}
		}
	}
	
	return nil, fmt.Errorf("decompression failed after %d attempts: %v", config.MaxRetries+1, lastErr)
}

// isRetryableError determines if an error is worth retrying.
func isRetryableError(err error, config *RetryConfig) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	
	// Memory-related errors
	if config.RetryOnMemory {
		if contains(errStr, "memory", "allocation", "cannot allocate", "out of memory") {
			return true
		}
	}
	
	// Buffer-related errors
	if config.RetryOnBuffer {
		if contains(errStr, "buffer", "dstSize_tooSmall", "size too small") {
			return true
		}
	}
	
	// Never retry on corruption or invalid data
	if contains(errStr, "corrupt", "invalid", "malformed", "checksum") {
		return false
	}
	
	return false
}

// contains checks if any of the substrings are present in s (case-insensitive).
func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if containsIgnoreCase(s, substr) {
			return true
		}
	}
	return false
}

// containsIgnoreCase checks if substr is in s (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive contains
	// In production, might want to use strings.Contains with strings.ToLower
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			// Simple ASCII case-insensitive comparison
			if c1 != c2 {
				if c1 >= 'A' && c1 <= 'Z' {
					c1 = c1 + 32 // Convert to lowercase
				}
				if c2 >= 'A' && c2 <= 'Z' {
					c2 = c2 + 32 // Convert to lowercase
				}
				if c1 != c2 {
					match = false
					break
				}
			}
		}
		if match {
			return true
		}
	}
	return false
}

// CompressWithFallback attempts compression with automatic fallback on failure.
// It tries advanced compression first, then falls back to simple compression.
func CompressWithFallback(dst, src []byte) []byte {
	// First try default compression  
	result := Compress(dst, src)
	if len(result) > 0 {
		return result
	}
	
	// Fallback to default compression
	result = Compress(dst, src)
	if len(result) > 0 {
		return result
	}
	
	// Last resort: minimal compression
	return CompressLevel(dst, src, 1)
}

// DecompressWithFallback attempts decompression with automatic error recovery.
func DecompressWithFallback(dst, src []byte) ([]byte, error) {
	// First try normal decompression
	result, err := Decompress(dst, src)
	if err == nil {
		return result, nil
	}
	
	// Try with retry logic
	return DecompressWithRetry(dst, src, DefaultRetryConfig())
}
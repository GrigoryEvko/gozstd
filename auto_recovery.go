package gozstd

/*
#include "zstd.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

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
				decompressedSize := int(C.ZSTD_getFrameContentSize(
					unsafe.Pointer(&src[0]), C.size_t(len(src))))
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

// SafeCompress attempts compression with automatic fallback on failure.
// It tries advanced compression first, then falls back to simple compression.
func SafeCompress(dst, src []byte) []byte {
	// First try auto-tuned compression
	result := CompressAuto(dst, src)
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

// SafeDecompress attempts decompression with automatic error recovery.
func SafeDecompress(dst, src []byte) ([]byte, error) {
	// First try normal decompression
	result, err := Decompress(dst, src)
	if err == nil {
		return result, nil
	}
	
	// Try with retry logic
	return DecompressWithRetry(dst, src, DefaultRetryConfig())
}
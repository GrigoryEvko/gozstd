package gozstd

import (
	"runtime"
	"sync/atomic"
)

// ManagedBuffer is a buffer that automatically returns to the pool when garbage collected.
// This helps prevent buffer leaks when users forget to return buffers.
type ManagedBuffer struct {
	data     []byte
	returned uint32 // atomic flag to prevent double-return
}

// NewManagedBuffer creates a new managed buffer with at least the specified capacity.
// The buffer will automatically return to the pool when garbage collected.
func NewManagedBuffer(minCapacity int) *ManagedBuffer {
	mb := &ManagedBuffer{
		data:     GetBuffer(minCapacity),
		returned: 0,
	}
	
	// Set finalizer for automatic cleanup
	runtime.SetFinalizer(mb, (*ManagedBuffer).autoReturn)
	return mb
}

// Bytes returns the underlying byte slice.
// The slice should not be used after Release() is called.
func (mb *ManagedBuffer) Bytes() []byte {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return nil
	}
	return mb.data
}

// Len returns the length of the buffer.
func (mb *ManagedBuffer) Len() int {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return 0
	}
	return len(mb.data)
}

// Cap returns the capacity of the buffer.
func (mb *ManagedBuffer) Cap() int {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return 0
	}
	return cap(mb.data)
}

// Resize resizes the buffer to the specified length.
// If the new length is greater than capacity, a new buffer is allocated.
func (mb *ManagedBuffer) Resize(newLen int) {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return
	}
	
	if newLen > cap(mb.data) {
		// Need a larger buffer
		newBuf := GetBuffer(newLen)
		copy(newBuf, mb.data)
		
		// Return old buffer to pool
		PutBuffer(mb.data)
		mb.data = newBuf[:newLen]
	} else {
		mb.data = mb.data[:newLen]
	}
}

// Append appends data to the buffer, growing it if necessary.
func (mb *ManagedBuffer) Append(p []byte) {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return
	}
	
	oldLen := len(mb.data)
	newLen := oldLen + len(p)
	
	if newLen > cap(mb.data) {
		// Need a larger buffer
		newBuf := GetBuffer(newLen)
		copy(newBuf, mb.data)
		
		// Return old buffer to pool
		PutBuffer(mb.data)
		mb.data = newBuf[:newLen]
	} else {
		mb.data = mb.data[:newLen]
	}
	
	copy(mb.data[oldLen:], p)
}

// Reset resets the buffer to zero length but keeps the capacity.
func (mb *ManagedBuffer) Reset() {
	if atomic.LoadUint32(&mb.returned) != 0 {
		return
	}
	mb.data = mb.data[:0]
}

// Release manually returns the buffer to the pool.
// The buffer should not be used after calling Release.
// This method is safe to call multiple times.
func (mb *ManagedBuffer) Release() {
	if !atomic.CompareAndSwapUint32(&mb.returned, 0, 1) {
		return // Already returned
	}
	
	// Clear finalizer since we're manually releasing
	runtime.SetFinalizer(mb, nil)
	
	// Return buffer to pool
	PutBuffer(mb.data)
	mb.data = nil
}

// autoReturn is called by the garbage collector to automatically return the buffer.
func (mb *ManagedBuffer) autoReturn() {
	if atomic.LoadUint32(&mb.returned) == 0 {
		mb.Release()
	}
}

// GetManagedBuffer is a convenience function that returns a managed buffer.
func GetManagedBuffer(minCapacity int) *ManagedBuffer {
	return NewManagedBuffer(minCapacity)
}

// GetManagedCompressBuffer returns a managed buffer suitable for compression.
func GetManagedCompressBuffer(inputSize int) *ManagedBuffer {
	estimatedSize := inputSize + (inputSize >> 8) + 64
	return NewManagedBuffer(estimatedSize)
}

// GetManagedDecompressBuffer returns a managed buffer suitable for decompression.
func GetManagedDecompressBuffer(hint int) *ManagedBuffer {
	if hint <= 0 {
		hint = 64 * 1024 // 64KB default
	}
	return NewManagedBuffer(hint)
}
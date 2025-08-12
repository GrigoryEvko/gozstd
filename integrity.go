package gozstd

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"sync"
)

// IntegrityConfig controls data integrity features
type IntegrityConfig struct {
	// EnableCRC32 adds CRC32 checksums
	EnableCRC32 bool
	
	// EnableSHA256 adds SHA256 checksums (more secure but slower)
	EnableSHA256 bool
	
	// VerifyOnDecompression automatically verifies checksums
	VerifyOnDecompression bool
	
	// AllowPartialRecovery attempts to recover partial data on corruption
	AllowPartialRecovery bool
}

// DefaultIntegrityConfig provides sensible defaults
var DefaultIntegrityConfig = &IntegrityConfig{
	EnableCRC32:           true,
	EnableSHA256:          false,
	VerifyOnDecompression: true,
	AllowPartialRecovery:  false,
}

// ChecksumType represents the type of checksum
type ChecksumType uint8

const (
	ChecksumNone ChecksumType = iota
	ChecksumCRC32
	ChecksumSHA256
)

// Frame magic numbers for identifying our wrapped format
const (
	IntegrityMagic = 0x5A535443 // "ZSTC" - Zstd with Checksum
	FrameVersion   = 1
)

// IntegrityFrame wraps compressed data with checksum
type IntegrityFrame struct {
	Magic    uint32       // Magic number
	Version  uint8        // Frame version
	Type     ChecksumType // Checksum type
	Checksum []byte       // Checksum value
	DataSize uint32       // Size of compressed data
	Data     []byte       // Compressed data
}

// CompressWithChecksum compresses data and adds checksum
func CompressWithChecksum(dst, src []byte) ([]byte, error) {
	return CompressWithChecksumConfig(dst, src, DefaultIntegrityConfig)
}

// CompressWithChecksumConfig compresses with custom integrity config
func CompressWithChecksumConfig(dst, src []byte, config *IntegrityConfig) ([]byte, error) {
	// Compress data first
	compressed := Compress(dst, src)
	
	// Calculate checksum
	var checksumType ChecksumType
	var checksum []byte
	
	if config.EnableSHA256 {
		checksumType = ChecksumSHA256
		h := sha256.Sum256(compressed)
		checksum = h[:]
	} else if config.EnableCRC32 {
		checksumType = ChecksumCRC32
		crc := crc32.ChecksumIEEE(compressed)
		checksum = make([]byte, 4)
		binary.LittleEndian.PutUint32(checksum, crc)
	} else {
		// No checksum, return compressed data as-is
		return compressed, nil
	}
	
	// Create frame
	frame := &IntegrityFrame{
		Magic:    IntegrityMagic,
		Version:  FrameVersion,
		Type:     checksumType,
		Checksum: checksum,
		DataSize: uint32(len(compressed)),
		Data:     compressed,
	}
	
	// Serialize frame
	return serializeFrame(frame)
}

// DecompressWithChecksum decompresses and verifies checksum
func DecompressWithChecksum(dst, src []byte) ([]byte, error) {
	return DecompressWithChecksumConfig(dst, src, DefaultIntegrityConfig)
}

// DecompressWithChecksumConfig decompresses with custom integrity config
func DecompressWithChecksumConfig(dst, src []byte, config *IntegrityConfig) ([]byte, error) {
	// Check if this is our integrity frame format
	if !hasIntegrityFrame(src) {
		// Regular compressed data without checksum
		return Decompress(dst, src)
	}
	
	// Parse frame
	frame, err := parseFrame(src)
	if err != nil {
		return nil, fmt.Errorf("failed to parse integrity frame: %w", err)
	}
	
	// Verify checksum if enabled
	if config.VerifyOnDecompression && frame.Type != ChecksumNone {
		if err := verifyChecksum(frame); err != nil {
			if !config.AllowPartialRecovery {
				return nil, fmt.Errorf("checksum verification failed: %w", err)
			}
			// Continue with partial recovery attempt
		}
	}
	
	// Decompress data
	return Decompress(dst, frame.Data)
}

// hasIntegrityFrame checks if data has our integrity frame
func hasIntegrityFrame(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	magic := binary.LittleEndian.Uint32(data[:4])
	return magic == IntegrityMagic
}

// serializeFrame serializes an integrity frame
func serializeFrame(frame *IntegrityFrame) ([]byte, error) {
	// Calculate total size
	totalSize := 4 + 1 + 1 + 4 + len(frame.Checksum) + 4 + len(frame.Data)
	result := make([]byte, totalSize)
	
	offset := 0
	
	// Write magic
	binary.LittleEndian.PutUint32(result[offset:], frame.Magic)
	offset += 4
	
	// Write version
	result[offset] = frame.Version
	offset++
	
	// Write checksum type
	result[offset] = uint8(frame.Type)
	offset++
	
	// Write checksum length
	binary.LittleEndian.PutUint32(result[offset:], uint32(len(frame.Checksum)))
	offset += 4
	
	// Write checksum
	copy(result[offset:], frame.Checksum)
	offset += len(frame.Checksum)
	
	// Write data size
	binary.LittleEndian.PutUint32(result[offset:], frame.DataSize)
	offset += 4
	
	// Write data
	copy(result[offset:], frame.Data)
	
	return result, nil
}

// parseFrame parses an integrity frame
func parseFrame(data []byte) (*IntegrityFrame, error) {
	if len(data) < 14 { // Minimum frame size
		return nil, errors.New("data too small to be an integrity frame")
	}
	
	frame := &IntegrityFrame{}
	offset := 0
	
	// Read magic
	frame.Magic = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	
	if frame.Magic != IntegrityMagic {
		return nil, fmt.Errorf("invalid magic number: 0x%x", frame.Magic)
	}
	
	// Read version
	frame.Version = data[offset]
	offset++
	
	if frame.Version != FrameVersion {
		return nil, fmt.Errorf("unsupported frame version: %d", frame.Version)
	}
	
	// Read checksum type
	frame.Type = ChecksumType(data[offset])
	offset++
	
	// Read checksum length
	checksumLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	
	if offset+int(checksumLen)+4 > len(data) {
		return nil, errors.New("invalid checksum length")
	}
	
	// Read checksum
	frame.Checksum = data[offset : offset+int(checksumLen)]
	offset += int(checksumLen)
	
	// Read data size
	frame.DataSize = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	
	if offset+int(frame.DataSize) > len(data) {
		return nil, errors.New("invalid data size")
	}
	
	// Read data
	frame.Data = data[offset : offset+int(frame.DataSize)]
	
	return frame, nil
}

// verifyChecksum verifies the checksum in a frame
func verifyChecksum(frame *IntegrityFrame) error {
	switch frame.Type {
	case ChecksumCRC32:
		if len(frame.Checksum) != 4 {
			return errors.New("invalid CRC32 checksum length")
		}
		expected := binary.LittleEndian.Uint32(frame.Checksum)
		actual := crc32.ChecksumIEEE(frame.Data)
		if expected != actual {
			return fmt.Errorf("CRC32 mismatch: expected 0x%x, got 0x%x", expected, actual)
		}
		
	case ChecksumSHA256:
		if len(frame.Checksum) != 32 {
			return errors.New("invalid SHA256 checksum length")
		}
		actual := sha256.Sum256(frame.Data)
		for i := range frame.Checksum {
			if frame.Checksum[i] != actual[i] {
				return fmt.Errorf("SHA256 mismatch")
			}
		}
		
	case ChecksumNone:
		// No checksum to verify
		
	default:
		return fmt.Errorf("unknown checksum type: %d", frame.Type)
	}
	
	return nil
}

// StreamingChecksum provides streaming checksum calculation
type StreamingChecksum struct {
	hasher hash.Hash
	writer io.Writer
	mu     sync.Mutex
}

// NewStreamingChecksum creates a streaming checksum calculator
func NewStreamingChecksum(w io.Writer, useSHA256 bool) *StreamingChecksum {
	var h hash.Hash
	if useSHA256 {
		h = sha256.New()
	} else {
		h = crc32.NewIEEE()
	}
	
	return &StreamingChecksum{
		hasher: h,
		writer: w,
	}
}

// Write implements io.Writer
func (sc *StreamingChecksum) Write(p []byte) (int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	// Update checksum
	sc.hasher.Write(p)
	
	// Write to underlying writer
	return sc.writer.Write(p)
}

// Sum returns the current checksum
func (sc *StreamingChecksum) Sum() []byte {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	return sc.hasher.Sum(nil)
}

// Reset resets the checksum calculation
func (sc *StreamingChecksum) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	sc.hasher.Reset()
}

// VerifyingReader wraps a reader with automatic checksum verification
type VerifyingReader struct {
	reader         io.Reader
	hasher         hash.Hash
	expectedSum    []byte
	verifyOnClose  bool
	bytesRead      int64
}

// NewVerifyingReader creates a reader that verifies checksum
func NewVerifyingReader(r io.Reader, expectedChecksum []byte) *VerifyingReader {
	var h hash.Hash
	if len(expectedChecksum) == 32 {
		h = sha256.New()
	} else if len(expectedChecksum) == 4 {
		h = crc32.NewIEEE()
	} else {
		// Unknown checksum type, disable verification
		return &VerifyingReader{
			reader: r,
		}
	}
	
	return &VerifyingReader{
		reader:        r,
		hasher:        h,
		expectedSum:   expectedChecksum,
		verifyOnClose: true,
	}
}

// Read implements io.Reader
func (vr *VerifyingReader) Read(p []byte) (int, error) {
	n, err := vr.reader.Read(p)
	if n > 0 && vr.hasher != nil {
		vr.hasher.Write(p[:n])
		vr.bytesRead += int64(n)
	}
	
	if err == io.EOF && vr.verifyOnClose {
		if verr := vr.Verify(); verr != nil {
			return n, verr
		}
	}
	
	return n, err
}

// Verify verifies the checksum
func (vr *VerifyingReader) Verify() error {
	if vr.hasher == nil || len(vr.expectedSum) == 0 {
		return nil // No verification needed
	}
	
	actualSum := vr.hasher.Sum(nil)
	
	if len(actualSum) != len(vr.expectedSum) {
		return fmt.Errorf("checksum length mismatch: expected %d, got %d",
			len(vr.expectedSum), len(actualSum))
	}
	
	for i := range actualSum {
		if actualSum[i] != vr.expectedSum[i] {
			return fmt.Errorf("checksum mismatch after reading %d bytes", vr.bytesRead)
		}
	}
	
	return nil
}

// CorruptionRecovery attempts to recover from corrupted compressed data
type CorruptionRecovery struct {
	MaxRetries     int
	SkipBadBlocks  bool
	PartialResults bool
}

// RecoverDecompression attempts to decompress corrupted data
func (cr *CorruptionRecovery) RecoverDecompression(dst, src []byte) ([]byte, error) {
	// Try standard decompression first
	result, err := Decompress(dst, src)
	if err == nil {
		return result, nil
	}
	
	// If corruption detected, try recovery strategies
	if cr.SkipBadBlocks {
		return cr.recoverWithBlockSkip(dst, src)
	}
	
	if cr.PartialResults {
		return cr.recoverPartial(dst, src)
	}
	
	return nil, fmt.Errorf("decompression failed and recovery not possible: %w", err)
}

// recoverWithBlockSkip tries to skip corrupted blocks
func (cr *CorruptionRecovery) recoverWithBlockSkip(dst, src []byte) ([]byte, error) {
	// This would implement block-level recovery
	// For now, return error
	return nil, errors.New("block skip recovery not yet implemented")
}

// recoverPartial tries to recover partial data
func (cr *CorruptionRecovery) recoverPartial(dst, src []byte) ([]byte, error) {
	// This would implement partial recovery
	// For now, return error
	return nil, errors.New("partial recovery not yet implemented")
}
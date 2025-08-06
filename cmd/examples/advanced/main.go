package main

import (
	"fmt"
	"log"

	"github.com/GrigoryEvko/gozstd"
)

func main() {
	// Sample data
	data := []byte("This example demonstrates the advanced compression API with custom parameters.")
	fmt.Printf("Original data: %s\n", data)
	fmt.Printf("Original size: %d bytes\n\n", len(data))

	// Create a compression context
	cctx := gozstd.NewCCtx()
	// Note: cctx cannot be released due to current library design

	// Set various compression parameters
	fmt.Println("Setting compression parameters:")

	// Set compression level
	err := cctx.SetParameter(gozstd.ZSTD_c_compressionLevel, 19)
	if err != nil {
		log.Fatalf("Failed to set compression level: %v", err)
	}
	fmt.Println("✓ Compression level: 19 (maximum)")

	// Enable checksum
	err = cctx.SetParameter(gozstd.ZSTD_c_checksumFlag, 1)
	if err != nil {
		log.Fatalf("Failed to enable checksum: %v", err)
	}
	fmt.Println("✓ Checksum: enabled")

	// Set window log
	err = cctx.SetParameter(gozstd.ZSTD_c_windowLog, 20)
	if err != nil {
		log.Fatalf("Failed to set window log: %v", err)
	}
	fmt.Println("✓ Window log: 20 (1MB window)")

	// Set strategy
	err = cctx.SetParameter(gozstd.ZSTD_c_strategy, int(gozstd.ZSTD_btultra2))
	if err != nil {
		log.Fatalf("Failed to set strategy: %v", err)
	}
	fmt.Println("✓ Strategy: btultra2 (best compression)")

	// Enable content size in frame header
	err = cctx.SetParameter(gozstd.ZSTD_c_contentSizeFlag, 1)
	if err != nil {
		log.Fatalf("Failed to enable content size flag: %v", err)
	}
	fmt.Println("✓ Content size in header: enabled")

	fmt.Println()

	// Compress with custom parameters
	compressed, err := cctx.Compress(nil, data)
	if err != nil {
		log.Fatalf("Compression failed: %v", err)
	}
	fmt.Printf("Compressed size: %d bytes\n", len(compressed))
	fmt.Printf("Compression ratio: %.2fx\n\n", float64(len(data))/float64(len(compressed)))

	// Decompress
	decompressed, err := gozstd.Decompress(nil, compressed)
	if err != nil {
		log.Fatalf("Decompression failed: %v", err)
	}

	// Verify the data matches
	if string(decompressed) == string(data) {
		fmt.Println("✓ Success: Data matches after advanced compression/decompression")
	} else {
		fmt.Println("✗ Error: Data mismatch")
	}

	// Demonstrate reset functionality
	fmt.Println("\n--- Demonstrating Context Reset ---")

	// Reset only the session (keeps parameters)
	err = cctx.Reset(gozstd.ZSTD_reset_session_only)
	if err != nil {
		log.Fatalf("Failed to reset session: %v", err)
	}
	fmt.Println("✓ Reset session (parameters retained)")

	// Compress again with same parameters
	compressed2, err := cctx.Compress(nil, []byte("Another compression with same parameters"))
	if err != nil {
		log.Fatalf("Second compression failed: %v", err)
	}
	fmt.Printf("Second compression size: %d bytes\n", len(compressed2))

	// Demonstrate pledged size
	fmt.Println("\n--- Demonstrating Pledged Size ---")

	// Reset session
	cctx.Reset(gozstd.ZSTD_reset_session_only)

	// Set pledged size (tells compressor the exact input size)
	pledgedData := []byte("Data with known size")
	err = cctx.SetPledgedSrcSize(uint64(len(pledgedData)))
	if err != nil {
		log.Fatalf("Failed to set pledged size: %v", err)
	}
	fmt.Printf("✓ Pledged size: %d bytes\n", len(pledgedData))

	// Compress with pledged size
	compressedPledged, err := cctx.Compress(nil, pledgedData)
	if err != nil {
		log.Fatalf("Compression with pledged size failed: %v", err)
	}
	fmt.Printf("Compressed with pledged size: %d bytes\n", len(compressedPledged))
	fmt.Println("\nPledged size can improve compression efficiency when input size is known")
}

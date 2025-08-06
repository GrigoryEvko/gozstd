package main

import (
	"fmt"
	"log"

	"github.com/GrigoryEvko/gozstd"
)

func main() {
	// Sample data to compress
	data := []byte("Hello, World! This is a simple compression example using gozstd.")
	fmt.Printf("Original data: %s\n", data)
	fmt.Printf("Original size: %d bytes\n\n", len(data))

	// Compress the data
	compressed := gozstd.Compress(nil, data)
	fmt.Printf("Compressed size: %d bytes\n", len(compressed))
	fmt.Printf("Compression ratio: %.2fx\n\n", float64(len(data))/float64(len(compressed)))

	// Decompress the data
	decompressed, err := gozstd.Decompress(nil, compressed)
	if err != nil {
		log.Fatalf("Decompression failed: %v", err)
	}

	fmt.Printf("Decompressed data: %s\n", decompressed)
	fmt.Printf("Decompressed size: %d bytes\n", len(decompressed))

	// Verify the data matches
	if string(decompressed) == string(data) {
		fmt.Println("\n✓ Success: Data matches after compression/decompression")
	} else {
		fmt.Println("\n✗ Error: Data mismatch after compression/decompression")
	}
}

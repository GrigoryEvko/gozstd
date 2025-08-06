package main

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/GrigoryEvko/gozstd"
)

func main() {
	// Large data to compress in streaming fashion
	data := bytes.Repeat([]byte("This is a test of streaming compression. "), 1000)
	fmt.Printf("Original size: %d bytes\n", len(data))

	// Compress using streaming
	var compressed bytes.Buffer
	writer := gozstd.NewWriter(&compressed)

	// Write data in chunks
	chunkSize := 1024
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]

		n, err := writer.Write(chunk)
		if err != nil {
			log.Fatalf("Write failed: %v", err)
		}
		if n != len(chunk) {
			log.Fatalf("Short write: %d != %d", n, len(chunk))
		}
	}

	// Close the writer to flush remaining data
	if err := writer.Close(); err != nil {
		log.Fatalf("Close failed: %v", err)
	}

	fmt.Printf("Compressed size: %d bytes\n", compressed.Len())
	fmt.Printf("Compression ratio: %.2fx\n\n", float64(len(data))/float64(compressed.Len()))

	// Decompress using streaming
	reader := gozstd.NewReader(&compressed)
	var decompressed bytes.Buffer

	// Read in chunks
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			decompressed.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Read failed: %v", err)
		}
	}

	fmt.Printf("Decompressed size: %d bytes\n", decompressed.Len())

	// Verify the data matches
	if bytes.Equal(decompressed.Bytes(), data) {
		fmt.Println("\n✓ Success: Data matches after streaming compression/decompression")
	} else {
		fmt.Println("\n✗ Error: Data mismatch after streaming compression/decompression")
	}
}

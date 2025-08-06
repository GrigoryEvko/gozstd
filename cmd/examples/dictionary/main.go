package main

import (
	"fmt"
	"log"

	"github.com/GrigoryEvko/gozstd"
)

func main() {
	// Sample data that shares common patterns
	samples := [][]byte{
		[]byte("The quick brown fox jumps over the lazy dog."),
		[]byte("The quick brown fox runs through the forest."),
		[]byte("The lazy dog sleeps under the tree."),
		[]byte("The brown fox is quick and clever."),
		[]byte("The dog is lazy but loyal."),
	}

	// Build a dictionary from samples
	dict := gozstd.BuildDict(samples, 1024) // 1KB dictionary
	if dict == nil {
		log.Fatal("Failed to build dictionary")
	}
	fmt.Printf("Dictionary size: %d bytes\n\n", len(dict))

	// Create compression and decompression dictionaries
	cd, err := gozstd.NewCDict(dict)
	if err != nil {
		log.Fatalf("Failed to create compression dictionary: %v", err)
	}
	defer cd.Release()

	dd, err := gozstd.NewDDict(dict)
	if err != nil {
		log.Fatalf("Failed to create decompression dictionary: %v", err)
	}
	defer dd.Release()

	// Test data similar to training samples
	testData := []byte("The quick brown fox and the lazy dog are friends.")
	fmt.Printf("Test data: %s\n", testData)
	fmt.Printf("Test data size: %d bytes\n\n", len(testData))

	// Compress with dictionary
	compressedWithDict := gozstd.CompressDict(nil, testData, cd)
	fmt.Printf("Compressed with dictionary: %d bytes\n", len(compressedWithDict))

	// Compress without dictionary for comparison
	compressedWithoutDict := gozstd.Compress(nil, testData)
	fmt.Printf("Compressed without dictionary: %d bytes\n", len(compressedWithoutDict))

	savings := len(compressedWithoutDict) - len(compressedWithDict)
	fmt.Printf("Dictionary savings: %d bytes (%.1f%%)\n\n",
		savings, float64(savings)/float64(len(compressedWithoutDict))*100)

	// Decompress with dictionary
	decompressed, err := gozstd.DecompressDict(nil, compressedWithDict, dd)
	if err != nil {
		log.Fatalf("Decompression failed: %v", err)
	}

	fmt.Printf("Decompressed: %s\n", decompressed)

	// Verify the data matches
	if string(decompressed) == string(testData) {
		fmt.Println("\n✓ Success: Dictionary compression/decompression works correctly")
	} else {
		fmt.Println("\n✗ Error: Data mismatch after dictionary compression/decompression")
	}

	// Demonstrate memory-efficient ByRef dictionary
	fmt.Println("\n--- Using ByRef Dictionary (memory-efficient) ---")

	cdByRef, err := gozstd.NewCDictByRef(dict)
	if err != nil {
		log.Fatalf("Failed to create ByRef compression dictionary: %v", err)
	}
	defer cdByRef.Release()

	compressedByRef := gozstd.CompressDict(nil, testData, cdByRef)
	fmt.Printf("Compressed with ByRef dictionary: %d bytes\n", len(compressedByRef))
	fmt.Println("ByRef dictionaries avoid copying dictionary data, saving memory")
}

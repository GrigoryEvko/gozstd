package gozstd

// This file consolidates example tests from:
// - gozstd_example_test.go
// - writer_example_test.go
// - reader_example_test.go
// - dict_example_test.go

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
)

// =============================================================================
// MERGED FROM gozstd_example_test.go
// =============================================================================

func ExampleCompress_simple() {
	data := []byte("foo bar baz")

	// Compress and decompress data into new buffers.
	compressedData := Compress(nil, data)
	decompressedData, err := Decompress(nil, compressedData)
	if err != nil {
		log.Fatalf("cannot decompress data: %s", err)
	}

	fmt.Printf("%s", decompressedData)
	// Output:
	// foo bar baz
}

func ExampleCompress_multiple() {
	data := []byte("foo bar baz")

	// Compress and decompress data into new buffers.
	compressedData := Compress(nil, data)
	decompressedData, err := Decompress(nil, append(compressedData, compressedData...))
	if err != nil {
		log.Fatalf("cannot decompress data: %s", err)
	}

	fmt.Printf("%s", decompressedData)
	// Output:
	// foo bar bazfoo bar baz
}

func ExampleDecompress_simple() {
	data := []byte("foo bar baz")

	// Compress and decompress data into new buffers.
	compressedData := Compress(nil, data)
	decompressedData, err := Decompress(nil, compressedData)
	if err != nil {
		log.Fatalf("cannot decompress data: %s", err)
	}

	fmt.Printf("%s", decompressedData)
	// Output:
	// foo bar baz
}

func ExampleCompress_noAllocs() {
	data := []byte("foo bar baz")

	// Compressed data will be put into cbuf.
	var cbuf []byte

	for i := 0; i < 5; i++ {
		// Compress re-uses cbuf for the compressed data.
		cbuf = Compress(cbuf[:0], data)

		decompressedData, err := Decompress(nil, cbuf)
		if err != nil {
			log.Fatalf("cannot decompress data: %s", err)
		}

		fmt.Printf("%d. %s\n", i, decompressedData)
	}

	// Output:
	// 0. foo bar baz
	// 1. foo bar baz
	// 2. foo bar baz
	// 3. foo bar baz
	// 4. foo bar baz
}

func ExampleDecompress_noAllocs() {
	data := []byte("foo bar baz")

	compressedData := Compress(nil, data)

	// Decompressed data will be put into dbuf.
	var dbuf []byte

	for i := 0; i < 5; i++ {
		// Decompress re-uses dbuf for the decompressed data.
		var err error
		dbuf, err = Decompress(dbuf[:0], compressedData)
		if err != nil {
			log.Fatalf("cannot decompress data: %s", err)
		}

		fmt.Printf("%d. %s\n", i, dbuf)
	}

	// Output:
	// 0. foo bar baz
	// 1. foo bar baz
	// 2. foo bar baz
	// 3. foo bar baz
	// 4. foo bar baz
}

// =============================================================================
// MERGED FROM writer_example_test.go
// =============================================================================

func ExampleWriter() {

	// Compress data to bb.
	var bb bytes.Buffer
	zw := NewWriter(&bb)
	defer zw.Release()

	for i := 0; i < 3; i++ {
		fmt.Fprintf(zw, "line %d\n", i)
	}
	if err := zw.Close(); err != nil {
		log.Fatalf("cannot close writer: %s", err)
	}

	// Decompress the data and verify it is valid.
	plainData, err := Decompress(nil, bb.Bytes())
	fmt.Printf("err: %v\n%s", err, plainData)

	// Output:
	// err: <nil>
	// line 0
	// line 1
	// line 2
}

func ExampleWriter_Flush() {

	var bb bytes.Buffer
	zw := NewWriter(&bb)
	defer zw.Release()

	// Write some data to zw.
	data := []byte("some data\nto compress")
	if _, err := zw.Write(data); err != nil {
		log.Fatalf("cannot write data to zw: %s", err)
	}

	// Verify the data is cached in zw and isn't propagated to bb.
	if bb.Len() > 0 {
		log.Fatalf("%d bytes unexpectedly propagated to bb", bb.Len())
	}

	// Flush the compressed data to bb.
	if err := zw.Flush(); err != nil {
		log.Fatalf("cannot flush compressed data: %s", err)
	}

	// Verify the compressed data is propagated to bb.
	if bb.Len() == 0 {
		log.Fatalf("the compressed data isn't propagated to bb")
	}

	// Try reading the compressed data with reader.
	zr := NewReader(&bb)
	defer zr.Release()
	buf := make([]byte, len(data))
	if _, err := io.ReadFull(zr, buf); err != nil {
		log.Fatalf("cannot read the compressed data: %s", err)
	}
	fmt.Printf("%s", buf)

	// Output:
	// some data
	// to compress
}

func ExampleWriter_Reset() {
	zw := NewWriter(nil)
	defer zw.Release()

	// Write to different destinations using the same Writer.
	for i := 0; i < 3; i++ {
		var bb bytes.Buffer
		zw.Reset(&bb, nil, DefaultCompressionLevel)
		if _, err := zw.Write([]byte(fmt.Sprintf("line %d", i))); err != nil {
			log.Fatalf("unexpected error when writing data: %s", err)
		}
		if err := zw.Close(); err != nil {
			log.Fatalf("unexpected error when closing zw: %s", err)
		}

		// Decompress the compressed data.
		plainData, err := Decompress(nil, bb.Bytes())
		if err != nil {
			log.Fatalf("unexpected error when decompressing data: %s", err)
		}
		fmt.Printf("%s\n", plainData)
	}

	// Output:
	// line 0
	// line 1
	// line 2
}

func ExampleWriterParams() {

	// Compress data to bb.
	var bb bytes.Buffer
	zw := NewWriterParams(&bb, &WriterParams{
		CompressionLevel: 10,
		WindowLog:        14,
	})
	defer zw.Release()

	for i := 0; i < 3; i++ {
		fmt.Fprintf(zw, "line %d\n", i)
	}
	if err := zw.Close(); err != nil {
		log.Fatalf("cannot close writer: %s", err)
	}

	// Decompress the data and verify it is valid.
	plainData, err := Decompress(nil, bb.Bytes())
	fmt.Printf("err: %v\n%s", err, plainData)

	// Output:
	// err: <nil>
	// line 0
	// line 1
	// line 2
}

// =============================================================================
// MERGED FROM reader_example_test.go
// =============================================================================

func ExampleReader() {
	// Compress the data.
	compressedData := Compress(nil, []byte("line 0\nline 1\nline 2"))

	// Read it via Reader.
	r := bytes.NewReader(compressedData)
	zr := NewReader(r)
	defer zr.Release()

	var a []int
	for i := 0; i < 3; i++ {
		var n int
		if _, err := fmt.Fscanf(zr, "line %d\n", &n); err != nil {
			log.Fatalf("cannot read line: %s", err)
		}
		a = append(a, n)
	}

	// Make sure there are no data left in zr.
	buf := make([]byte, 1)
	if _, err := zr.Read(buf); err != io.EOF {
		log.Fatalf("unexpected error; got %v; want %v", err, io.EOF)
	}

	fmt.Println(a)

	// Output:
	// [0 1 2]
}

func ExampleReader_Reset() {
	zr := NewReader(nil)
	defer zr.Release()

	// Read from different sources using the same Reader.
	for i := 0; i < 3; i++ {
		compressedData := Compress(nil, []byte(fmt.Sprintf("line %d", i)))
		r := bytes.NewReader(compressedData)
		zr.Reset(r, nil)

		data, err := ioutil.ReadAll(zr)
		if err != nil {
			log.Fatalf("unexpected error when reading compressed data: %s", err)
		}
		fmt.Printf("%s\n", data)
	}

	// Output:
	// line 0
	// line 1
	// line 2
}

// =============================================================================
// MERGED FROM dict_example_test.go
// =============================================================================

func ExampleBuildDict() {
	// Collect samples for the dictionary.
	var samples [][]byte
	for i := 0; i < 1000; i++ {
		sample := fmt.Sprintf("this is a dict sample number %d", i)
		samples = append(samples, []byte(sample))
	}

	// Build a dictionary with the desired size of 8Kb.
	dict := BuildDict(samples, 8*1024)

	// Now the dict may be used for compression/decompression.

	// Create CDict from the dict.
	cd, err := NewCDict(dict)
	if err != nil {
		log.Fatalf("cannot create CDict: %s", err)
	}
	defer cd.Release()

	// Compress multiple blocks with the same CDict.
	var compressedBlocks [][]byte
	for i := 0; i < 3; i++ {
		plainData := fmt.Sprintf("this is line %d for dict compression", i)
		compressedData := CompressDict(nil, []byte(plainData), cd)
		compressedBlocks = append(compressedBlocks, compressedData)
	}

	// The compressedData must be decompressed with the same dict.

	// Create DDict from the dict.
	dd, err := NewDDict(dict)
	if err != nil {
		log.Fatalf("cannot create DDict: %s", err)
	}
	defer dd.Release()

	// Decompress multiple blocks with the same DDict.
	for _, compressedData := range compressedBlocks {
		decompressedData, err := DecompressDict(nil, compressedData, dd)
		if err != nil {
			log.Fatalf("cannot decompress data: %s", err)
		}
		fmt.Printf("%s\n", decompressedData)
	}

	// Output:
	// this is line 0 for dict compression
	// this is line 1 for dict compression
	// this is line 2 for dict compression
}
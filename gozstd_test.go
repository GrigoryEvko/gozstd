package gozstd

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDecompressSmallBlockWithoutSingleSegmentFlag(t *testing.T) {
	// See https://github.com/VictoriaMetrics/VictoriaMetrics/issues/281 for details.
	cblockHex := "28B52FFD00007D000038C0A907DFD40300015407022B0E02"
	dblockHexExpected := "C0A907DFD4030000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000000000000" +
		"000000000000000000000000000000000"

	cblock := mustUnhex(cblockHex)
	dblockExpected := mustUnhex(dblockHexExpected)

	t.Run("empty-dst-buf", func(t *testing.T) {
		dblock, err := Decompress(nil, cblock)
		if err != nil {
			t.Fatalf("unexpected error when decrompressing with empty initial buffer: %s", err)
		}
		if string(dblock) != string(dblockExpected) {
			t.Fatalf("unexpected decompressed block;\ngot\n%X\nwant\n%X", dblock, dblockExpected)
		}
	})
	t.Run("small-dst-buf", func(t *testing.T) {
		buf := make([]byte, len(dblockExpected)/2)
		dblock, err := Decompress(buf[:0], cblock)
		if err != nil {
			t.Fatalf("unexpected error when decrompressing with empty initial buffer: %s", err)
		}
		if string(dblock) != string(dblockExpected) {
			t.Fatalf("unexpected decompressed block;\ngot\n%X\nwant\n%X", dblock, dblockExpected)
		}
	})
	t.Run("enough-dst-buf", func(t *testing.T) {
		buf := make([]byte, len(dblockExpected))
		dblock, err := Decompress(buf[:0], cblock)
		if err != nil {
			t.Fatalf("unexpected error when decrompressing with empty initial buffer: %s", err)
		}
		if string(dblock) != string(dblockExpected) {
			t.Fatalf("unexpected decompressed block;\ngot\n%X\nwant\n%X", dblock, dblockExpected)
		}
	})
}

func mustUnhex(dataHex string) []byte {
	data, err := hex.DecodeString(dataHex)
	if err != nil {
		panic(fmt.Errorf("BUG: cannot unhex %q: %s", dataHex, err))
	}
	return data
}

func TestCompressDecompressDistinctConcurrentDicts(t *testing.T) {
	// Build multiple distinct dicts.
	var cdicts []*CDict
	var ddicts []*DDict
	defer func() {
		for _, cd := range cdicts {
			cd.Release()
		}
		for _, dd := range ddicts {
			dd.Release()
		}
	}()
	for i := 0; i < 4; i++ {
		var samples [][]byte
		for j := 0; j < 1000; j++ {
			sample := fmt.Sprintf("this is %d,%d sample", j, i)
			samples = append(samples, []byte(sample))
		}
		dict := BuildDict(samples, 4*1024)
		cd, err := NewCDict(dict)
		if err != nil {
			t.Fatalf("cannot create CDict: %s", err)
		}
		cdicts = append(cdicts, cd)
		dd, err := NewDDict(dict)
		if err != nil {
			t.Fatalf("cannot create DDict: %s", err)
		}
		ddicts = append(ddicts, dd)
	}

	// Build data for the compression.
	var bb bytes.Buffer
	i := 0
	for bb.Len() < 1e4 {
		fmt.Fprintf(&bb, "%d sample line this is %d", bb.Len(), i)
		i++
	}
	data := bb.Bytes()

	// Run concurrent goroutines compressing/decompressing with distinct dicts.
	ch := make(chan error, len(cdicts))
	for i := 0; i < cap(ch); i++ {
		go func(cd *CDict, dd *DDict) {
			ch <- testCompressDecompressDistinctConcurrentDicts(cd, dd, data)
		}(cdicts[i], ddicts[i])
	}

	// Wait for goroutines to finish.
	for i := 0; i < cap(ch); i++ {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
	}
}

func testCompressDecompressDistinctConcurrentDicts(cd *CDict, dd *DDict, data []byte) error {
	var compressedData, decompressedData []byte
	for j := 0; j < 10; j++ {
		compressedData = CompressDict(compressedData[:0], data, cd)

		var err error
		decompressedData, err = DecompressDict(decompressedData[:0], compressedData, dd)
		if err != nil {
			return fmt.Errorf("cannot decompress data: %s", err)
		}
		if !bytes.Equal(decompressedData, data) {
			return fmt.Errorf("unexpected decompressed data; got\n%q; want\n%q", decompressedData, data)
		}
	}
	return nil
}

func TestCompressDecompressDict(t *testing.T) {
	var samples [][]byte
	for i := 0; i < 1000; i++ {
		sample := fmt.Sprintf("%d this is line %d", i, i)
		samples = append(samples, []byte(sample))
	}
	dict := BuildDict(samples, 16*1024)

	cd, err := NewCDict(dict)
	if err != nil {
		t.Fatalf("cannot create CDict: %s", err)
	}
	defer cd.Release()
	dd, err := NewDDict(dict)
	if err != nil {
		t.Fatalf("cannot create DDict: %s", err)
	}
	defer dd.Release()

	// Run serial test.
	if err := testCompressDecompressDictSerial(cd, dd); err != nil {
		t.Fatalf("error in serial test: %s", err)
	}

	// Run concurrent test.
	ch := make(chan error, 5)
	for i := 0; i < cap(ch); i++ {
		go func() {
			ch <- testCompressDecompressDictSerial(cd, dd)
		}()
	}
	for i := 0; i < cap(ch); i++ {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("error in concurrent test: %s", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout in concurrent test")
		}
	}
}

func testCompressDecompressDictSerial(cd *CDict, dd *DDict) error {
	for i := 0; i < 30; i++ {
		var src []byte
		for j := 0; j < 100; j++ {
			src = append(src, []byte(fmt.Sprintf("line %d is this %d\n", j, i+j))...)
		}
		compressedData := CompressDict(nil, src, cd)
		plainData, err := DecompressDict(nil, compressedData, dd)
		if err != nil {
			return fmt.Errorf("unexpected error when decompressing %d bytes: %s", len(src), err)
		}
		if string(plainData) != string(src) {
			return fmt.Errorf("unexpected data after decompressing %d bytes; got\n%X; want\n%X", len(src), plainData, src)
		}

		// Try decompressing without dict.
		_, err = Decompress(nil, compressedData)
		if err == nil {
			return fmt.Errorf("expecting non-nil error when decompressing without dict")
		}
		if !strings.Contains(err.Error(), "Dictionary mismatch") {
			return fmt.Errorf("unexpected error when decompressing without dict: %q; must contain %q", err, "Dictionary mismatch")
		}
	}
	return nil
}

func TestDecompressInvalidData(t *testing.T) {
	// Try decompressing invalid data.
	src := []byte("invalid compressed data")
	buf := make([]byte, len(src))
	if _, err := Decompress(nil, src); err == nil {
		t.Fatalf("expecting error when decompressing invalid data")
	}
	if _, err := Decompress(buf[:0], src); err == nil {
		t.Fatalf("expecting error when decompressing invalid data into existing buffer")
	}

	// Try decompressing corrupted data.
	s := newTestString(64*1024, 15)
	cd := Compress(nil, []byte(s))
	cd[len(cd)-1]++

	if _, err := Decompress(nil, cd); err == nil {
		t.Fatalf("expecting error when decompressing corrupted data")
	}
	if _, err := Decompress(buf[:0], cd); err == nil {
		t.Fatalf("expecting error when decompressing corrupdate data into existing buffer")
	}
}

func TestCompressLevel(t *testing.T) {
	src := []byte("foobar baz")

	for compressLevel := 1; compressLevel < 22; compressLevel++ {
		testCompressLevel(t, src, compressLevel)
	}

	// Test invalid compression levels - they should clamp
	// to the closest valid levels.
	testCompressLevel(t, src, -123)
	testCompressLevel(t, src, 234324)
}

func testCompressLevel(t *testing.T, src []byte, compressionLevel int) {
	t.Helper()

	cd := CompressLevel(nil, src, compressionLevel)
	dd, err := Decompress(nil, cd)
	if err != nil {
		t.Fatalf("unexpected error during decompression: %s", err)
	}
	if string(dd) != string(src) {
		t.Fatalf("unexpected dd\n%X; want\n%X", dd, src)
	}
}

func TestCompressDecompress(t *testing.T) {
	testCompressDecompress(t, "")
	testCompressDecompress(t, "a")
	testCompressDecompress(t, "foo bar")

	for size := 1; size <= 1e6; size *= 10 {
		s := newTestString(size, 20)
		testCompressDecompress(t, s)
	}
}

func testCompressDecompress(t *testing.T, s string) {
	t.Helper()

	if err := testCompressDecompressSerial(s); err != nil {
		t.Fatalf("error in serial test: %s", err)
	}

	ch := make(chan error, runtime.GOMAXPROCS(-1)+2)
	for i := 0; i < cap(ch); i++ {
		go func() {
			ch <- testCompressDecompressSerial(s)
		}()
	}
	for i := 0; i < cap(ch); i++ {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("unexpected error in parallel test: %s", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout in parallel test")
		}
	}
}

func testCompressDecompressSerial(s string) error {
	cs := Compress(nil, []byte(s))
	ds, err := Decompress(nil, cs)
	if err != nil {
		return fmt.Errorf("cannot decompress: %s\ns=%X\ncs=%X", err, s, cs)
	}
	if string(ds) != s {
		return fmt.Errorf("unexpected ds (len=%d, sLen=%d, cslen=%d)\n%X; want\n%X", len(ds), len(s), len(cs), ds, s)
	}

	// Verify prefixed decompression.
	prefix := []byte("foobaraaa")
	ds, err = Decompress(prefix, cs)
	if err != nil {
		return fmt.Errorf("cannot decompress prefixed cs: %s\ns=%X\ncs=%X", err, s, cs)
	}
	if string(ds[:len(prefix)]) != string(prefix) {
		return fmt.Errorf("unexpected prefix in the decompressed result: %X; want %X", ds[:len(prefix)], prefix)
	}
	ds = ds[len(prefix):]
	if string(ds) != s {
		return fmt.Errorf("unexpected prefixed ds\n%X; want\n%X", ds, s)
	}

	// Verify prefixed compression.
	csp := Compress(prefix, []byte(s))
	if string(csp[:len(prefix)]) != string(prefix) {
		return fmt.Errorf("unexpected prefix in the compressed result: %X; want %X", csp[:len(prefix)], prefix)
	}
	csp = csp[len(prefix):]
	if string(csp) != string(cs) {
		return fmt.Errorf("unexpected prefixed cs\n%X; want\n%X", csp, cs)
	}
	return nil
}

func newTestString(size, randomness int) string {
	s := make([]byte, size)
	for i := 0; i < size; i++ {
		s[i] = byte(rand.Intn(randomness))
	}
	return string(s)
}

func TestCompressDecompressMultiFrames(t *testing.T) {
	var bb bytes.Buffer
	for bb.Len() < 3*128*1024 {
		fmt.Fprintf(&bb, "compress/decompress big data %d, ", bb.Len())
	}
	origData := append([]byte{}, bb.Bytes()...)

	cd := Compress(nil, bb.Bytes())
	plainData, err := Decompress(nil, cd)
	if err != nil {
		t.Fatalf("cannot decompress big data: %s", err)
	}
	if !bytes.Equal(plainData, origData) {
		t.Fatalf("unexpected data decompressed: got\n%q; want\n%q\nlen(data)=%d, len(orig)=%d",
			plainData, origData, len(plainData), len(origData))
	}
}

func TestCCtxSetParams(t *testing.T) {
	ctx := NewCCtx()
	err := ctx.SetParameter(ZSTD_c_compressionLevel, 0)
	if err != nil {
		t.Fatalf("cannot set parameter: compressionLevel")
	}
	err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
	if err != nil {
		t.Fatalf("cannot set parameter: checksumFlag")
	}
}

func TestCompress2(t *testing.T) {
	var bb bytes.Buffer
	for bb.Len() < 3*128*1024 {
		fmt.Fprintf(&bb, "compress/decompress big data %d, ", bb.Len())
	}
	origData := append([]byte{}, bb.Bytes()...)

	ctx := NewCCtx()
	err := ctx.SetParameter(ZSTD_c_compressionLevel, 0)
	if err != nil {
		t.Fatalf("cannot set parameter: compressionLevel")
	}
	err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
	if err != nil {
		t.Fatalf("cannot set parameter: checksumFlag")
	}

	cd, err := ctx.Compress(nil, bb.Bytes())
	if err != nil {
		t.Fatalf("cannot ctx.Compress: %s", err)
	}

	plainData, err := Decompress(nil, cd)
	if err != nil {
		t.Fatalf("cannot decompress big data: %s", err)
	}
	if !bytes.Equal(plainData, origData) {
		t.Fatalf("unexpected data decompressed: got\n%q; want\n%q\nlen(data)=%d, len(orig)=%d",
			plainData, origData, len(plainData), len(origData))
	}
}

func TestAdvancedAPIAllParameters(t *testing.T) {
	testData := []byte("This is test data for advanced compression API testing. " +
		"We need some repetitive data to test compression effectively. " +
		"Repetitive data repetitive data repetitive data repetitive data.")

	tests := []struct {
		name       string
		parameters map[CParameter]int
	}{
		{
			name: "compression_levels",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 5,
			},
		},
		{
			name: "window_log",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_windowLog:        15,
			},
		},
		{
			name: "hash_log",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_hashLog:          20,
			},
		},
		{
			name: "chain_log",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_chainLog:         20,
			},
		},
		{
			name: "search_log",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_searchLog:        5,
			},
		},
		{
			name: "min_match",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_minMatch:         4,
			},
		},
		{
			name: "target_length",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_targetLength:     64,
			},
		},
		{
			name: "checksum_enabled",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_checksumFlag:     1,
			},
		},
		{
			name: "checksum_disabled",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_checksumFlag:     0,
			},
		},
		{
			name: "content_size_flag",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_contentSizeFlag:  1,
			},
		},
		{
			name: "dict_id_flag",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 3,
				ZSTD_c_dictIDFlag:       1,
			},
		},
		{
			name: "ldm_enabled",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel:           3,
				ZSTD_c_enableLongDistanceMatching: 1,
				ZSTD_c_ldmHashLog:                 20,
				ZSTD_c_ldmMinMatch:                64,
			},
		},
		{
			name: "multi_params",
			parameters: map[CParameter]int{
				ZSTD_c_compressionLevel: 5,
				ZSTD_c_windowLog:        20,
				ZSTD_c_hashLog:          18,
				ZSTD_c_checksumFlag:     1,
				ZSTD_c_contentSizeFlag:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			// Apply all parameters
			for param, value := range tt.parameters {
				err := ctx.SetParameter(param, value)
				if err != nil {
					t.Fatalf("cannot set parameter %v to %v: %s", param, value, err)
				}
			}

			// Compress
			compressed, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("compression failed: %s", err)
			}

			// Decompress
			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("decompression failed: %s", err)
			}

			// Verify
			if !bytes.Equal(decompressed, testData) {
				t.Fatalf("data mismatch: got %q, want %q", decompressed, testData)
			}
		})
	}
}

func TestAdvancedAPIStrategies(t *testing.T) {
	testData := []byte(strings.Repeat("Test data for compression strategies. ", 100))

	strategies := []struct {
		name     string
		strategy ZSTD_CompressionStrategy
	}{
		{"fast", ZSTD_fast},
		{"dfast", ZSTD_dfast},
		{"greedy", ZSTD_greedy},
		{"lazy", ZSTD_lazy},
		{"lazy2", ZSTD_lazy2},
		{"btlazy2", ZSTD_btlazy2},
		{"btopt", ZSTD_btopt},
		{"btultra", ZSTD_btultra},
		{"btultra2", ZSTD_btultra2},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			err := ctx.SetParameter(ZSTD_c_compressionLevel, 3)
			if err != nil {
				t.Fatalf("cannot set compression level: %s", err)
			}
			
			err = ctx.SetParameter(ZSTD_c_strategy, int(s.strategy))
			if err != nil {
				t.Fatalf("cannot set strategy %s: %s", s.name, err)
			}

			compressed, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("compression with strategy %s failed: %s", s.name, err)
			}

			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("decompression failed: %s", err)
			}

			if !bytes.Equal(decompressed, testData) {
				t.Fatalf("data mismatch with strategy %s", s.name)
			}
		})
	}
}

func TestAdvancedAPIReset(t *testing.T) {
	testData := []byte("Test data for reset functionality")

	resetDirectives := []struct {
		name      string
		directive ZSTD_ResetDirective
	}{
		{"session_only", ZSTD_reset_session_only},
		{"parameters", ZSTD_reset_parameters},
		{"session_and_parameters", ZSTD_reset_session_and_parameters},
	}

	for _, rd := range resetDirectives {
		t.Run(rd.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			// Set some parameters
			ctx.SetParameter(ZSTD_c_compressionLevel, 5)
			ctx.SetParameter(ZSTD_c_checksumFlag, 1)
			
			// Compress once
			compressed1, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("first compression failed: %s", err)
			}

			// Reset
			err = ctx.Reset(rd.directive)
			if err != nil {
				t.Fatalf("reset with %s failed: %s", rd.name, err)
			}

			// Compress again
			compressed2, err := ctx.Compress(nil, testData)
			if err != nil {
				t.Fatalf("second compression failed: %s", err)
			}

			// Both should decompress correctly
			decompressed1, err := Decompress(nil, compressed1)
			if err != nil {
				t.Fatalf("first decompression failed: %s", err)
			}
			
			decompressed2, err := Decompress(nil, compressed2)
			if err != nil {
				t.Fatalf("second decompression failed: %s", err)
			}

			if !bytes.Equal(decompressed1, testData) || !bytes.Equal(decompressed2, testData) {
				t.Fatalf("data mismatch after reset")
			}
		})
	}
}

func TestAdvancedAPIPledgedSrcSize(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		pledgedSize  uint64
		expectError  bool
	}{
		{
			name:        "exact_size",
			data:        []byte("Hello, World!"),
			pledgedSize: 13,
			expectError: false,
		},
		{
			name:        "zero_size",
			data:        []byte{},
			pledgedSize: 0,
			expectError: false,
		},
		{
			name:        "large_data",
			data:        bytes.Repeat([]byte("Large data "), 1000),
			pledgedSize: 11000,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			err := ctx.SetPledgedSrcSize(tt.pledgedSize)
			if err != nil {
				t.Fatalf("cannot set pledged size: %s", err)
			}

			compressed, err := ctx.Compress(nil, tt.data)
			if (err != nil) != tt.expectError {
				t.Fatalf("compression error mismatch: got %v, expectError %v", err, tt.expectError)
			}

			if !tt.expectError {
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					t.Fatalf("decompression failed: %s", err)
				}

				if !bytes.Equal(decompressed, tt.data) {
					t.Fatalf("data mismatch")
				}
			}
		})
	}
}

func TestAdvancedAPIMultiThreading(t *testing.T) {
	// Note: Multi-threading parameters require ZSTD_MULTITHREAD=1 during compilation
	// These tests verify the API works but may not actually use multiple threads
	// if the library wasn't compiled with multi-threading support
	
	largeData := bytes.Repeat([]byte("Multi-threading test data. "), 10000)

	tests := []struct {
		name       string
		nbWorkers  int
		jobSize    int
		overlapLog int
	}{
		{
			name:      "single_worker",
			nbWorkers: 1,
		},
		{
			name:      "two_workers",
			nbWorkers: 2,
		},
		{
			name:      "four_workers_with_jobsize",
			nbWorkers: 4,
			jobSize:   1048576, // 1MB
		},
		{
			name:       "workers_with_overlap",
			nbWorkers:  2,
			overlapLog: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			// Set multi-threading parameters
			err := ctx.SetParameter(ZSTD_c_nbWorkers, tt.nbWorkers)
			if err != nil {
				// Multi-threading might not be available
				t.Skipf("cannot set nbWorkers: %s (multi-threading might not be compiled in)", err)
			}

			if tt.jobSize > 0 {
				err = ctx.SetParameter(ZSTD_c_jobSize, tt.jobSize)
				if err != nil {
					t.Fatalf("cannot set jobSize: %s", err)
				}
			}

			if tt.overlapLog > 0 {
				err = ctx.SetParameter(ZSTD_c_overlapLog, tt.overlapLog)
				if err != nil {
					t.Fatalf("cannot set overlapLog: %s", err)
				}
			}

			compressed, err := ctx.Compress(nil, largeData)
			if err != nil {
				t.Fatalf("compression failed: %s", err)
			}

			decompressed, err := Decompress(nil, compressed)
			if err != nil {
				t.Fatalf("decompression failed: %s", err)
			}

			if !bytes.Equal(decompressed, largeData) {
				t.Fatalf("data mismatch")
			}
		})
	}
}

func TestAdvancedAPIErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		param       CParameter
		value       int
		expectError bool
	}{
		{
			name:        "negative_compression_level",
			param:       ZSTD_c_compressionLevel,
			value:       -5,
			expectError: false, // Negative levels are allowed
		},
		{
			name:        "too_high_compression_level",
			param:       ZSTD_c_compressionLevel,
			value:       25,
			expectError: false, // Will be clamped to max
		},
		{
			name:        "invalid_strategy",
			param:       ZSTD_c_strategy,
			value:       999,
			expectError: true,
		},
		{
			name:        "invalid_checksum_flag",
			param:       ZSTD_c_checksumFlag,
			value:       2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewCCtx()
			
			err := ctx.SetParameter(tt.param, tt.value)
			if (err != nil) != tt.expectError {
				t.Fatalf("error mismatch: got %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

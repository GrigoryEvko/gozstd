package gozstd

// This file consolidates all benchmark tests from:
// - gozstd_bench_test.go
// - reader_bench_test.go
// - writer_bench_test.go
// - stream_bench_test.go
// - corpus_bench_test.go

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// Global sink to prevent optimizations
var Sink uint64

const benchBlocksPerStream = 10

// ===== From gozstd_bench_test.go =====

func BenchmarkDecompressDict(b *testing.B) {
	var samples [][]byte
	for i := 0; i < 4; i++ {
		sample := newBenchDataDictionary(false)
		compressedSample := Compress(nil, sample)
		samples = append(samples, compressedSample)
	}
	dict := BuildDict(samples, 64*1024)

	cd, err := NewCDict(dict)
	if err != nil {
		b.Fatalf("cannot create CDict: %s", err)
	}
	defer cd.Release()

	dd, err := NewDDict(dict)
	if err != nil {
		b.Fatalf("cannot create DDict: %s", err)
	}
	defer dd.Release()

	src := newBenchDataDictionary(false)
	compressedData := CompressDict(nil, src, cd)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			dst, err := DecompressDict(nil, compressedData, dd)
			if err != nil {
				panic(fmt.Errorf("unexpected error: %s", err))
			}
			n += len(dst)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

func BenchmarkCompressDict(b *testing.B) {
	var samples [][]byte
	for i := 0; i < 4; i++ {
		sample := newBenchDataDictionary(false)
		compressedSample := Compress(nil, sample)
		samples = append(samples, compressedSample)
	}
	dict := BuildDict(samples, 64*1024)

	cd, err := NewCDict(dict)
	if err != nil {
		b.Fatalf("cannot create CDict: %s", err)
	}
	defer cd.Release()

	src := newBenchDataDictionary(false)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			dst := CompressDict(nil, src, cd)
			n += len(dst)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

func BenchmarkCompress(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkCompress(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkCompress(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			dst := CompressLevel(nil, src, level)
			n += len(dst)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

func BenchmarkDecompress(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkDecompress(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkDecompress(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)
	compressedData := CompressLevel(nil, src, level)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := 0
		for pb.Next() {
			dst, err := Decompress(nil, compressedData)
			if err != nil {
				panic(fmt.Errorf("unexpected error: %s", err))
			}
			n += len(dst)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

var benchBlockSizes = []int{1024, 16 * 1024, 256 * 1024, 16 * 1024 * 1024}

func newBenchData(blockSize int) []byte {
	r := rand.New(rand.NewSource(1))
	data := make([]byte, blockSize)
	n, err := r.Read(data)
	if err != nil {
		panic(fmt.Errorf("cannot read random data: %s", err))
	}
	if n != blockSize {
		panic(fmt.Errorf("read %d bytes; want %d bytes", n, blockSize))
	}
	return data
}

func newBenchDataDictionary(b bool) []byte {
	var data []byte
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		data = append(data, []byte(fmt.Sprintf("data_%d", r.Intn(32)))...)
		if b {
			data = append(data, []byte(fmt.Sprintf("data_%d", r.Intn(16)))...)
		}
	}
	return data
}

// ===== From reader_bench_test.go =====

func BenchmarkReaderDict(b *testing.B) {
	var samples [][]byte
	for i := 0; i < 4; i++ {
		sample := newBenchDataDictionary(i < 2)
		compressedSample := Compress(nil, sample)
		samples = append(samples, compressedSample)
	}
	dict := BuildDict(samples, 64*1024)

	dd, err := NewDDict(dict)
	if err != nil {
		b.Fatalf("cannot create DDict: %s", err)
	}
	defer dd.Release()

	src := newBenchDataDictionary(false)
	cd := bytes.NewReader(CompressDict(nil, src, mustNewCDict(dict)))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(nil)
		var buf [16 * 1024]byte
		zr := NewReaderDict(r, dd)
		defer zr.Release()
		n := 0
		for pb.Next() {
			for {
				nn, err := zr.Read(buf[:])
				n += nn
				if err != nil {
					if err == io.EOF {
						break
					}
					panic(fmt.Errorf("unexpected error: %s", err))
				}
			}
			r.Reset(cd.Bytes())
			zr.Reset(r, nil)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

func BenchmarkReader(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkReader(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkReader(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)
	cd := bytes.NewReader(CompressLevel(nil, src, level))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(nil)
		var buf [16 * 1024]byte
		zr := NewReader(r)
		defer zr.Release()
		n := 0
		for pb.Next() {
			for {
				nn, err := zr.Read(buf[:])
				n += nn
				if err != nil {
					if err == io.EOF {
						break
					}
					panic(fmt.Errorf("unexpected error: %s", err))
				}
			}
			r.Reset(cd.Bytes())
			zr.Reset(r, nil)
		}
		atomic.AddUint64(&Sink, uint64(n))
	})
}

// ===== From writer_bench_test.go =====

func BenchmarkWriter(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkWriter(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkWriter(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		zw := NewWriterLevel(nil, level)
		defer zw.Release()
		bb := newBytesBuffer(len(src) * benchBlocksPerStream)
		for pb.Next() {
			bb.Reset()
			zw.Reset(bb, nil, 0)
			for i := 0; i < benchBlocksPerStream; i++ {
				if _, err := zw.Write(src); err != nil {
					panic(fmt.Errorf("unexpected error: %s", err))
				}
			}
			if err := zw.Close(); err != nil {
				panic(fmt.Errorf("unexpected error: %s", err))
			}
		}
	})
}

func BenchmarkWriter_Params(b *testing.B) {
	params := &WriterParams{
		WindowLog: 15,
	}
	zw := NewWriter(ioutil.Discard)
	defer zw.Release()

	for n := 0; n < b.N; n++ {
		zw.Reset(ioutil.Discard, nil, 0)
		zw.ResetWriterParams(ioutil.Discard, params)
	}
}

// ===== From stream_bench_test.go =====

func BenchmarkStreamCompress(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkStreamCompress(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkStreamCompress(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)
	cd := bytes.Repeat(src, benchBlocksPerStream)

	b.ReportAllocs()
	b.SetBytes(int64(len(cd)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(cd)
		var bb bytesBuffer
		for pb.Next() {
			bb.Reset()
			if err := StreamCompressLevel(&bb, r, level); err != nil {
				panic(fmt.Errorf("unexpected error: %s", err))
			}
			r.Reset(cd)
		}
	})
}

func BenchmarkStreamDecompress(b *testing.B) {
	for _, blockSize := range benchBlockSizes {
		b.Run(fmt.Sprintf("block-size-%d", blockSize), func(b *testing.B) {
			for level := 1; level <= 5; level++ {
				b.Run(fmt.Sprintf("level-%d", level), func(b *testing.B) {
					benchmarkStreamDecompress(b, blockSize, level)
				})
			}
		})
	}
}

func benchmarkStreamDecompress(b *testing.B, blockSize, level int) {
	src := newBenchData(blockSize)
	data := bytes.Repeat(src, benchBlocksPerStream)
	var bb bytesBuffer
	if err := StreamCompressLevel(&bb, bytes.NewReader(data), level); err != nil {
		b.Fatalf("cannot compress stream: %s", err)
	}
	cd := bb.B

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(cd)
		for pb.Next() {
			if err := StreamDecompress(ioutil.Discard, r); err != nil {
				panic(fmt.Errorf("unexpected error: %s", err))
			}
			r.Reset(cd)
		}
	})
}

// ===== From corpus_bench_test.go =====

// downloadCorpus downloads common compression test files if needed
func downloadCorpus(b *testing.B) string {
	corpusDir := filepath.Join(os.TempDir(), "zstd-corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		b.Logf("Downloading test corpus to %s", corpusDir)
		if err := os.MkdirAll(corpusDir, 0755); err != nil {
			b.Skip("Cannot create corpus directory")
		}
		
		// Download some common test files
		files := []struct {
			name string
			url  string
		}{
			{"silesia.tar", "https://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip"},
			{"enwik8", "https://mattmahoney.net/dc/enwik8.zip"},
		}
		
		for _, f := range files {
			filepath := filepath.Join(corpusDir, f.name)
			if _, err := os.Stat(filepath); os.IsNotExist(err) {
				// Use wget or curl to download
				cmd := exec.Command("wget", "-q", "-O", filepath+".zip", f.url)
				if err := cmd.Run(); err != nil {
					cmd = exec.Command("curl", "-s", "-o", filepath+".zip", f.url)
					if err := cmd.Run(); err != nil {
						b.Skip("Cannot download corpus files")
					}
				}
				// Unzip
				cmd = exec.Command("unzip", "-q", "-o", filepath+".zip", "-d", corpusDir)
				if err := cmd.Run(); err != nil {
					b.Skip("Cannot unzip corpus files")
				}
			}
		}
	}
	return corpusDir
}

func BenchmarkCorpusRawVsWrapper(b *testing.B) {
	corpusDir := downloadCorpus(b)
	
	// Read test files
	files, err := os.ReadDir(corpusDir)
	if err != nil {
		b.Skip("Cannot read corpus directory")
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		path := filepath.Join(corpusDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		
		if len(data) > 100*1024*1024 { // Skip files larger than 100MB
			continue
		}
		
		b.Run(file.Name(), func(b *testing.B) {
			b.Run("Raw", func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ResetTimer()
				
				for i := 0; i < b.N; i++ {
					compressed := Compress(nil, data)
					if len(compressed) == 0 {
						b.Fatal("Compression failed")
					}
				}
			})
			
			b.Run("Wrapper", func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ResetTimer()
				
				for i := 0; i < b.N; i++ {
					var buf bytes.Buffer
					w := NewWriter(&buf)
					n, err := w.Write(data)
					if err != nil || n != len(data) {
						b.Fatal("Write failed")
					}
					if err := w.Close(); err != nil {
						b.Fatal("Close failed")
					}
					w.Release()
				}
			})
		})
	}
}

func BenchmarkCorpusDecompression(b *testing.B) {
	corpusDir := downloadCorpus(b)
	
	files, err := os.ReadDir(corpusDir)
	if err != nil {
		b.Skip("Cannot read corpus directory")
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		path := filepath.Join(corpusDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		
		if len(data) > 100*1024*1024 {
			continue
		}
		
		compressed := CompressLevel(nil, data, 3)
		
		b.Run(file.Name(), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					b.Fatal("Decompression failed")
				}
				if len(decompressed) != len(data) {
					b.Fatal("Size mismatch")
				}
			}
		})
	}
}

func BenchmarkCorpusStrategies(b *testing.B) {
	corpusDir := downloadCorpus(b)
	
	files, err := os.ReadDir(corpusDir)
	if err != nil {
		b.Skip("Cannot read corpus directory")
	}
	
	strategies := []struct {
		name  string
		level int
	}{
		{"Fast", 1},
		{"Default", 3},
		{"Better", 6},
		{"Best", 9},
		{"Ultra", 19},
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		path := filepath.Join(corpusDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		
		if len(data) > 10*1024*1024 { // Use smaller limit for strategy tests
			continue
		}
		
		b.Run(file.Name(), func(b *testing.B) {
			for _, s := range strategies {
				b.Run(s.name, func(b *testing.B) {
					b.SetBytes(int64(len(data)))
					
					// Track compression ratio
					compressed := CompressLevel(nil, data, s.level)
					ratio := float64(len(data)) / float64(len(compressed))
					b.ReportMetric(ratio, "ratio")
					
					// Measure compression speed
					b.ResetTimer()
					var totalCompressed int64
					for i := 0; i < b.N; i++ {
						compressed := CompressLevel(nil, data, s.level)
						totalCompressed += int64(len(compressed))
					}
					
					// Report average compressed size
					avgCompressed := totalCompressed / int64(b.N)
					ratio = float64(len(data)) / float64(avgCompressed)
					b.ReportMetric(ratio, "ratio")
				})
			}
		})
	}
}

// ===== Helper types =====

type bytesBuffer struct {
	B []byte
}

func newBytesBuffer(capacity int) *bytesBuffer {
	return &bytesBuffer{
		B: make([]byte, 0, capacity),
	}
}

func (bb *bytesBuffer) Write(p []byte) (int, error) {
	bb.B = append(bb.B, p...)
	return len(p), nil
}

func (bb *bytesBuffer) Reset() {
	bb.B = bb.B[:0]
}

// mustNewCDict creates a new CDict and panics on error
func mustNewCDict(dict []byte) *CDict {
	cd, err := NewCDict(dict)
	if err != nil {
		panic(fmt.Errorf("cannot create CDict: %s", err))
	}
	return cd
}

// Lock to prevent corpus downloads from running in parallel
var corpusDownloadMu sync.Mutex
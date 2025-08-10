package gozstd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// Global corpus data cache for benchmarks
var (
	corpusDataCache map[string][]byte
	corpusCacheMu   sync.Mutex
	corpusOnce      sync.Once
)

// initCorpusCache downloads and caches corpus data for benchmarks
func initCorpusCache(b *testing.B) {
	corpusOnce.Do(func() {
		// Skip if git is not available
		if _, err := exec.LookPath("git"); err != nil {
			b.Skip("git not found in PATH, skipping corpus benchmarks")
		}

		// Download corpus
		tmpDir := downloadCorpus(b)
		defer os.RemoveAll(tmpDir)

		// Load all files into memory
		corpusDataCache = make(map[string][]byte)
		for _, cf := range corpusFiles {
			zipPath := filepath.Join(tmpDir, cf.zipName)
			data, err := extractZipFile(zipPath)
			if err != nil {
				b.Fatalf("Failed to extract %s: %v", cf.zipName, err)
			}
			corpusDataCache[cf.name] = data
		}

		b.Logf("Corpus cache initialized with %d files", len(corpusDataCache))
	})
}

// BenchmarkCorpusRawVsWrapper compares raw ZSTD with our wrapper
func BenchmarkCorpusRawVsWrapper(b *testing.B) {
	initCorpusCache(b)

	// Test subset of files for reasonable benchmark time
	testFiles := []string{"dickens", "mozilla", "xml", "samba"}
	compressionLevels := []int{1, 3, 5, 9}

	for _, fileName := range testFiles {
		data := corpusDataCache[fileName]
		if data == nil {
			b.Fatalf("File %s not found in cache", fileName)
		}

		b.Run(fileName, func(b *testing.B) {
			for _, level := range compressionLevels {
				b.Run(fmt.Sprintf("level_%d", level), func(b *testing.B) {
					b.Run("raw", func(b *testing.B) {
						benchmarkRawCompression(b, data, level)
					})
					b.Run("wrapper", func(b *testing.B) {
						benchmarkWrapperCompression(b, data, level)
					})
					b.Run("advanced", func(b *testing.B) {
						benchmarkAdvancedCompression(b, data, level)
					})
				})
			}
		})
	}
}

func benchmarkRawCompression(b *testing.B, data []byte, level int) {
	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	var totalCompressed int64
	for i := 0; i < b.N; i++ {
		compressed, err := compressRaw(data, level)
		if err != nil {
			b.Fatalf("Raw compression failed: %v", err)
		}
		totalCompressed += int64(len(compressed))
	}

	avgCompressed := totalCompressed / int64(b.N)
	ratio := float64(len(data)) / float64(avgCompressed)
	b.ReportMetric(ratio, "ratio")
}

func benchmarkWrapperCompression(b *testing.B, data []byte, level int) {
	// Pre-allocate to avoid allocations in the loop
	// Use a generous estimate for compressed size
	dst := make([]byte, 0, len(data)+1024)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	var totalCompressed int64
	for i := 0; i < b.N; i++ {
		compressed := CompressLevel(dst[:0], data, level)
		totalCompressed += int64(len(compressed))
	}

	avgCompressed := totalCompressed / int64(b.N)
	ratio := float64(len(data)) / float64(avgCompressed)
	b.ReportMetric(ratio, "ratio")
}

func benchmarkAdvancedCompression(b *testing.B, data []byte, level int) {
	ctx := NewCCtx()
	defer ctx.Release()
	err := ctx.SetParameter(ZSTD_c_compressionLevel, level)
	if err != nil {
		b.Fatalf("Failed to set compression level: %v", err)
	}

	// Pre-allocate to avoid allocations in the loop
	// Use a generous estimate for compressed size
	dst := make([]byte, 0, len(data)+1024)

	b.ResetTimer()
	b.SetBytes(int64(len(data)))

	var totalCompressed int64
	for i := 0; i < b.N; i++ {
		compressed, err := ctx.Compress(dst[:0], data)
		if err != nil {
			b.Fatalf("Advanced compression failed: %v", err)
		}
		totalCompressed += int64(len(compressed))
	}

	avgCompressed := totalCompressed / int64(b.N)
	ratio := float64(len(data)) / float64(avgCompressed)
	b.ReportMetric(ratio, "ratio")
}

// BenchmarkCorpusDecompression benchmarks decompression performance
func BenchmarkCorpusDecompression(b *testing.B) {
	initCorpusCache(b)

	// Pre-compress data at different levels
	testFiles := []string{"dickens", "mozilla", "xml"}
	compressionLevels := []int{1, 5, 19}

	type compressedData struct {
		raw     []byte
		wrapper []byte
	}

	precompressed := make(map[string]map[int]compressedData)

	for _, fileName := range testFiles {
		data := corpusDataCache[fileName]
		if data == nil {
			b.Fatalf("File %s not found in cache", fileName)
		}

		precompressed[fileName] = make(map[int]compressedData)

		for _, level := range compressionLevels {
			raw, err := compressRaw(data, level)
			if err != nil {
				b.Fatalf("Failed to compress %s: %v", fileName, err)
			}

			wrapper := CompressLevel(nil, data, level)

			precompressed[fileName][level] = compressedData{
				raw:     raw,
				wrapper: wrapper,
			}
		}
	}

	// Benchmark decompression
	for _, fileName := range testFiles {
		origData := corpusDataCache[fileName]

		b.Run(fileName, func(b *testing.B) {
			for _, level := range compressionLevels {
				cd := precompressed[fileName][level]

				b.Run(fmt.Sprintf("level_%d", level), func(b *testing.B) {
					b.Run("raw", func(b *testing.B) {
						benchmarkRawDecompression(b, cd.raw, len(origData))
					})
					b.Run("wrapper", func(b *testing.B) {
						benchmarkWrapperDecompression(b, cd.wrapper, len(origData))
					})
				})
			}
		})
	}
}

func benchmarkRawDecompression(b *testing.B, compressed []byte, origSize int) {
	b.ResetTimer()
	b.SetBytes(int64(origSize))

	for i := 0; i < b.N; i++ {
		decompressed, err := decompressRaw(compressed)
		if err != nil {
			b.Fatalf("Raw decompression failed: %v", err)
		}
		_ = decompressed
	}
}

func benchmarkWrapperDecompression(b *testing.B, compressed []byte, origSize int) {
	// Pre-allocate to avoid allocations
	dst := make([]byte, 0, origSize)

	b.ResetTimer()
	b.SetBytes(int64(origSize))

	for i := 0; i < b.N; i++ {
		decompressed, err := Decompress(dst[:0], compressed)
		if err != nil {
			b.Fatalf("Wrapper decompression failed: %v", err)
		}
		_ = decompressed
	}
}

// BenchmarkCorpusStrategies compares different compression strategies
func BenchmarkCorpusStrategies(b *testing.B) {
	initCorpusCache(b)

	// Test text vs binary files
	testCases := []struct {
		fileName string
		fileType string
	}{
		{"dickens", "text"},
		{"xml", "structured_text"},
		{"mozilla", "binary"},
		{"sao", "binary_data"},
	}

	strategies := []struct {
		name     string
		strategy ZSTD_CompressionStrategy
	}{
		{"fast", ZSTD_fast},
		{"dfast", ZSTD_dfast},
		{"greedy", ZSTD_greedy},
		{"lazy2", ZSTD_lazy2},
		{"btopt", ZSTD_btopt},
	}

	for _, tc := range testCases {
		data := corpusDataCache[tc.fileName]
		if data == nil {
			b.Fatalf("File %s not found in cache", tc.fileName)
		}

		b.Run(fmt.Sprintf("%s_%s", tc.fileType, tc.fileName), func(b *testing.B) {
			for _, s := range strategies {
				b.Run(s.name, func(b *testing.B) {
					ctx := NewCCtx()
					defer ctx.Release()
					ctx.SetParameter(ZSTD_c_compressionLevel, 3)
					ctx.SetParameter(ZSTD_c_strategy, int(s.strategy))

					// Pre-allocate with generous estimate
					dst := make([]byte, 0, len(data)+1024)

					b.ResetTimer()
					b.SetBytes(int64(len(data)))

					var totalCompressed int64
					for i := 0; i < b.N; i++ {
						compressed, err := ctx.Compress(dst[:0], data)
						if err != nil {
							b.Fatalf("Compression failed: %v", err)
						}
						totalCompressed += int64(len(compressed))
					}

					avgCompressed := totalCompressed / int64(b.N)
					ratio := float64(len(data)) / float64(avgCompressed)
					b.ReportMetric(ratio, "ratio")
				})
			}
		})
	}
}

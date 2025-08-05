package gozstd

import (
	"fmt"
	"testing"
)

// BenchmarkAdvancedFeatures benchmarks newly implemented advanced ZSTD features
func BenchmarkAdvancedFeatures(b *testing.B) {
	// Test data sizes
	blockSizes := []int{1e3, 1e4, 1e5}
	levels := []int{3, 5, 9}

	for _, blockSize := range blockSizes {
		data := newBenchString(blockSize)
		
		b.Run(fmt.Sprintf("blockSize_%d", blockSize), func(b *testing.B) {
			for _, level := range levels {
				b.Run(fmt.Sprintf("level_%d", level), func(b *testing.B) {
					
					// Benchmark Rsync-friendly compression
					b.Run("RsyncFriendly", func(b *testing.B) {
						cctx := NewCCtx()
						defer cctx.Release()
						
						// Enable rsync-friendly mode with multi-threading
						if err := cctx.SetParameter(ZSTD_c_nbWorkers, 2); err != nil {
							b.Fatalf("failed to set nbWorkers: %v", err)
						}
						if err := cctx.SetRsyncFriendly(true); err != nil {
							b.Fatalf("failed to set rsync friendly: %v", err)
						}
						if err := cctx.SetParameter(ZSTD_c_compressionLevel, level); err != nil {
							b.Fatalf("failed to set compression level: %v", err)
						}
						
						b.ResetTimer()
						b.SetBytes(int64(blockSize))
						b.ReportAllocs()
						
						for i := 0; i < b.N; i++ {
							compressed, err := cctx.Compress(nil, data)
							if err != nil {
								b.Fatalf("compression failed: %v", err)
							}
							// Prevent optimization
							if len(compressed) == 0 {
								b.Fatal("empty compression result")
							}
						}
					})

					// Benchmark Advanced Streaming Parameters
					b.Run("AdvancedStreaming", func(b *testing.B) {
						cctx := NewCCtx()
						defer cctx.Release()
						
						// Enable advanced streaming parameters
						cctx.SetSourceSizeHint(uint64(len(data)))
						cctx.SetStableInputBuffer(true)
						cctx.SetStableOutputBuffer(true)
						cctx.SetParameter(ZSTD_c_compressionLevel, level)
						
						b.ResetTimer()
						b.SetBytes(int64(blockSize))
						b.ReportAllocs()
						
						for i := 0; i < b.N; i++ {
							compressed, err := cctx.Compress(nil, data)
							if err != nil {
								b.Fatalf("compression failed: %v", err)
							}
							if len(compressed) == 0 {
								b.Fatal("empty compression result")
							}
						}
					})

					// Benchmark External Sequence Producer
					b.Run("SequenceProducer", func(b *testing.B) {
						cctx := NewCCtx()
						defer cctx.Release()
						
						// Register simple sequence producer
						err := cctx.RegisterSequenceProducer(SimpleSequenceProducer)
						if err != nil {
							b.Fatalf("failed to register sequence producer: %v", err)
						}
						defer cctx.ClearSequenceProducer()
						
						cctx.SetParameter(ZSTD_c_compressionLevel, level)
						cctx.SetSequenceValidation(true)
						cctx.SetSequenceProducerFallback(true)
						
						b.ResetTimer()
						b.SetBytes(int64(blockSize))
						b.ReportAllocs()
						
						for i := 0; i < b.N; i++ {
							compressed, err := cctx.Compress(nil, data)
							if err != nil {
								b.Fatalf("compression failed: %v", err)
							}
							if len(compressed) == 0 {
								b.Fatal("empty compression result")
							}
						}
					})

					// Benchmark Frame Inspection
					b.Run("FrameInspection", func(b *testing.B) {
						// Pre-compress data for inspection
						compressed := CompressLevel(nil, data, level)
						
						b.ResetTimer()
						b.SetBytes(int64(len(compressed)))
						b.ReportAllocs()
						
						for i := 0; i < b.N; i++ {
							// Test all frame inspection functions
							size, err := GetFrameContentSize(compressed)
							if err != nil {
								b.Fatalf("GetFrameContentSize failed: %v", err)
							}
							if size != uint64(len(data)) {
								b.Fatalf("size mismatch: got %d, want %d", size, len(data))
							}
							
							compSize, err := GetFrameCompressedSize(compressed)
							if err != nil {
								b.Fatalf("GetFrameCompressedSize failed: %v", err)
							}
							if compSize != uint64(len(compressed)) {
								b.Fatalf("compressed size mismatch: got %d, want %d", compSize, len(compressed))
							}
							
							// Legacy function (should be same as GetFrameContentSize)
							legacySize := GetDecompressedSize(compressed)
							if legacySize != uint64(len(data)) {
								b.Fatalf("legacy size mismatch: got %d, want %d", legacySize, len(data))
							}
						}
					})
				})
			}
		})
	}
}

// BenchmarkEnhancedDictionaryPerformance benchmarks the advanced dictionary training algorithms
func BenchmarkEnhancedDictionaryPerformance(b *testing.B) {
	// Create training samples
	samples := createBenchmarkSamples(100, 1000) // 100 samples, 1KB each
	dictSize := 4096

	b.Run("FastCover", func(b *testing.B) {
		params := DefaultFastCoverParams()
		params.NotificationLevel = 0
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dict, err := BuildDictFastCover(samples, dictSize, params)
			if err != nil {
				b.Fatalf("FastCover training failed: %v", err)
			}
			if len(dict) == 0 {
				b.Fatal("empty dictionary")
			}
		}
	})

	b.Run("FastCoverOptimized", func(b *testing.B) {
		params := DefaultFastCoverParams()
		params.K = 0 // Enable optimization
		params.D = 0 // Enable optimization
		params.Steps = 5 // Fewer steps for benchmarking
		params.NotificationLevel = 0
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result, err := BuildDictFastCoverOptimized(samples, dictSize, params)
			if err != nil {
				b.Fatalf("FastCover optimization failed: %v", err)
			}
			if len(result.Dictionary) == 0 {
				b.Fatal("empty dictionary")
			}
		}
	})

	b.Run("Cover", func(b *testing.B) {
		params := DefaultCoverParams()
		params.NotificationLevel = 0
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dict, err := BuildDictCover(samples, dictSize, params)
			if err != nil {
				b.Fatalf("Cover training failed: %v", err)
			}
			if len(dict) == 0 {
				b.Fatal("empty dictionary")
			}
		}
	})
}

// createBenchmarkSamples creates samples for dictionary training benchmarks
func createBenchmarkSamples(numSamples, sampleSize int) [][]byte {
	samples := make([][]byte, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = newBenchString(sampleSize)
	}
	return samples
}
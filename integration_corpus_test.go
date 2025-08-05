package gozstd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// corpusFiles contains the expected files in the Silesia Corpus
var corpusFiles = []struct {
	name        string
	zipName     string
	description string
}{
	{"dickens", "dickens.zip", "English novels (text)"},
	{"mozilla", "mozilla.zip", "Program executables"},
	{"mr", "mr.zip", "Medical images (DICOM)"},
	{"nci", "nci.zip", "Chemical database (text)"},
	{"ooffice", "ooffice.zip", "Windows DLL"},
	{"osdb", "osdb.zip", "Database (binary)"},
	{"reymont", "reymont.zip", "Polish text (PDF)"},
	{"samba", "samba.zip", "Source code and graphics"},
	{"sao", "sao.zip", "Star catalog (binary)"},
	{"webster", "webster.zip", "English dictionary (HTML)"},
	{"x-ray", "x-ray.zip", "Medical image (DICOM)"},
	{"xml", "xml.zip", "XML files (text)"},
}

// downloadCorpus clones the Silesia Corpus to a temporary directory
func downloadCorpus(t testing.TB) string {
	t.Helper()
	
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "silesia-corpus-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Clone the repository
	cmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/MiloszKrajewski/SilesiaCorpus.git", tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to clone corpus: %v\nOutput: %s", err, output)
	}
	
	return tmpDir
}

// extractZipFile extracts a single file from a zip archive
func extractZipFile(zipPath string) ([]byte, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()
	
	// Silesia corpus zips contain a single file
	if len(r.File) != 1 {
		return nil, fmt.Errorf("expected 1 file in zip, got %d", len(r.File))
	}
	
	f := r.File[0]
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file in zip: %w", err)
	}
	defer rc.Close()
	
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	return data, nil
}


func TestCorpusCompression(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping corpus tests")
	}
	
	// Download corpus
	corpusDir := downloadCorpus(t)
	defer os.RemoveAll(corpusDir)
	
	compressionLevels := []int{1, 3, 5, 9, 19}
	
	for _, cf := range corpusFiles {
		t.Run(cf.name, func(t *testing.T) {
			// Extract file from zip
			zipPath := filepath.Join(corpusDir, cf.zipName)
			data, err := extractZipFile(zipPath)
			if err != nil {
				t.Fatalf("Failed to extract %s: %v", cf.zipName, err)
			}
			
			t.Logf("Testing %s (%s): %d bytes", cf.name, cf.description, len(data))
			
			for _, level := range compressionLevels {
				t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
					// Measure raw ZSTD compression time
					rawStart := time.Now()
					rawCompressed, err := compressRaw(data, level)
					rawDuration := time.Since(rawStart)
					if err != nil {
						t.Fatalf("Raw compression failed: %v", err)
					}
					
					// Measure wrapper compression time
					wrapperStart := time.Now()
					wrapperCompressed := CompressLevel(nil, data, level)
					wrapperDuration := time.Since(wrapperStart)
					
					// Calculate compression ratios and speeds
					rawRatio := float64(len(data)) / float64(len(rawCompressed))
					wrapperRatio := float64(len(data)) / float64(len(wrapperCompressed))
					
					// Calculate MB/s throughput
					rawMBps := float64(len(data)) / (1024 * 1024) / rawDuration.Seconds()
					wrapperMBps := float64(len(data)) / (1024 * 1024) / wrapperDuration.Seconds()
					
					t.Logf("Raw compressed: %d bytes (ratio: %.2fx) in %v (%.1f MB/s)", 
						len(rawCompressed), rawRatio, rawDuration, rawMBps)
					t.Logf("Wrapper compressed: %d bytes (ratio: %.2fx) in %v (%.1f MB/s)", 
						len(wrapperCompressed), wrapperRatio, wrapperDuration, wrapperMBps)
					
					// Calculate speed improvement
					if rawDuration > 0 {
						speedup := (rawDuration.Seconds() - wrapperDuration.Seconds()) / rawDuration.Seconds() * 100
						if speedup > 0 {
							t.Logf("Wrapper is %.1f%% faster than raw", speedup)
						} else {
							t.Logf("Wrapper is %.1f%% slower than raw", -speedup)
						}
					}
					
					// The compressed sizes might differ slightly, but decompressed data must match
					
					// Decompress raw compressed data
					rawDecompressed, err := decompressRaw(rawCompressed)
					if err != nil {
						t.Fatalf("Raw decompression failed: %v", err)
					}
					
					// Decompress wrapper compressed data
					wrapperDecompressed, err := Decompress(nil, wrapperCompressed)
					if err != nil {
						t.Fatalf("Wrapper decompression failed: %v", err)
					}
					
					// Verify decompressed data matches original
					if !bytes.Equal(rawDecompressed, data) {
						t.Errorf("Raw decompressed data doesn't match original")
					}
					if !bytes.Equal(wrapperDecompressed, data) {
						t.Errorf("Wrapper decompressed data doesn't match original")
					}
					
					// Cross-decompress: decompress raw with wrapper and vice versa
					crossDecompressed1, err := Decompress(nil, rawCompressed)
					if err != nil {
						t.Fatalf("Cross-decompression (raw->wrapper) failed: %v", err)
					}
					if !bytes.Equal(crossDecompressed1, data) {
						t.Errorf("Cross-decompressed data (raw->wrapper) doesn't match original")
					}
					
					crossDecompressed2, err := decompressRaw(wrapperCompressed)
					if err != nil {
						t.Fatalf("Cross-decompression (wrapper->raw) failed: %v", err)
					}
					if !bytes.Equal(crossDecompressed2, data) {
						t.Errorf("Cross-decompressed data (wrapper->raw) doesn't match original")
					}
				})
			}
		})
	}
}

func TestCorpusAdvancedAPI(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping corpus tests")
	}
	
	// Download corpus
	corpusDir := downloadCorpus(t)
	defer os.RemoveAll(corpusDir)
	
	// Test a subset of files with advanced API
	testFiles := []string{"dickens", "xml", "mozilla"}
	
	for _, fileName := range testFiles {
		t.Run(fileName, func(t *testing.T) {
			// Find the corresponding corpus file
			var cf struct {
				name        string
				zipName     string
				description string
			}
			for _, f := range corpusFiles {
				if f.name == fileName {
					cf = f
					break
				}
			}
			
			// Extract file
			zipPath := filepath.Join(corpusDir, cf.zipName)
			data, err := extractZipFile(zipPath)
			if err != nil {
				t.Fatalf("Failed to extract %s: %v", cf.zipName, err)
			}
			
			// Test with checksum enabled
			t.Run("with_checksum", func(t *testing.T) {
				ctx := NewCCtx()
				
				err := ctx.SetParameter(ZSTD_c_compressionLevel, 5)
				if err != nil {
					t.Fatalf("Failed to set compression level: %v", err)
				}
				
				err = ctx.SetParameter(ZSTD_c_checksumFlag, 1)
				if err != nil {
					t.Fatalf("Failed to enable checksum: %v", err)
				}
				
				compressed, err := ctx.Compress(nil, data)
				if err != nil {
					t.Fatalf("Advanced API compression failed: %v", err)
				}
				
				// Decompress and verify
				decompressed, err := Decompress(nil, compressed)
				if err != nil {
					t.Fatalf("Decompression failed: %v", err)
				}
				
				if !bytes.Equal(decompressed, data) {
					t.Errorf("Decompressed data doesn't match original")
				}
				
				t.Logf("Compressed %d bytes to %d bytes (ratio: %.2fx) with checksum",
					len(data), len(compressed), float64(len(data))/float64(len(compressed)))
			})
			
			// Test with different strategies
			strategies := []struct {
				name     string
				strategy ZSTD_CompressionStrategy
			}{
				{"fast", ZSTD_fast},
				{"lazy2", ZSTD_lazy2},
				{"btopt", ZSTD_btopt},
			}
			
			for _, s := range strategies {
				t.Run(fmt.Sprintf("strategy_%s", s.name), func(t *testing.T) {
					ctx := NewCCtx()
					
					ctx.SetParameter(ZSTD_c_compressionLevel, 3)
					ctx.SetParameter(ZSTD_c_strategy, int(s.strategy))
					
					compressed, err := ctx.Compress(nil, data)
					if err != nil {
						t.Fatalf("Compression with strategy %s failed: %v", s.name, err)
					}
					
					t.Logf("Strategy %s: compressed to %d bytes (ratio: %.2fx)",
						s.name, len(compressed), float64(len(data))/float64(len(compressed)))
				})
			}
		})
	}
}
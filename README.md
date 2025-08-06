[![Build Status](https://github.com/GrigoryEvko/gozstd/workflows/CI/badge.svg)](https://github.com/GrigoryEvko/gozstd/actions)
[![GoDoc](https://pkg.go.dev/badge/github.com/GrigoryEvko/gozstd)](https://pkg.go.dev/github.com/GrigoryEvko/gozstd)
[![Go Report](https://goreportcard.com/badge/github.com/GrigoryEvko/gozstd)](https://goreportcard.com/report/github.com/GrigoryEvko/gozstd)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/v/release/GrigoryEvko/gozstd)](https://github.com/GrigoryEvko/gozstd/releases)

# gozstd - High-Performance Go Wrapper for [Zstandard Compression](http://facebook.github.io/zstd/)

`gozstd` provides a high-performance, thread-safe Go wrapper for the Zstandard compression library with automatic multi-threading support and zero-allocation APIs.

## Table of Contents
- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Advanced Usage](#advanced-usage)
- [Performance](#performance)
- [API Documentation](#api-documentation)
- [File Organization](#file-organization)
- [Testing](#testing)
- [FAQ](#faq)
- [Contributing](#contributing)
- [Changelog](#changelog)

## Features

### Core Capabilities
- **Zero-modification vendoring** of upstream [zstd](https://github.com/facebook/zstd)
- **Optimized for speed** with zero-allocation API options
- **High concurrency** optimized `Compress*` and `Decompress*` functions
- **Proper streaming support** with [Writer.Flush](https://pkg.go.dev/github.com/GrigoryEvko/gozstd#Writer.Flush) for network applications

### Compression Features
- Block and stream compression/decompression
- All compression levels supported (1-22, including negative levels for fast compression)
- [Dictionary compression](https://github.com/facebook/zstd#the-case-for-small-data-compression) support
- Dictionary building from sample sets
- Persistent dictionary storage and network transfer support

### Advanced Features (v1.25.0+)
- **Automatic Global Thread Pool**: Improved performance with shared thread pool across all compression contexts
- **Enhanced Multi-threading Parameters**:
  - `NbWorkers`: Number of worker threads
  - `JobSize`: Size of jobs for parallel processing
  - `OverlapLog`: Amount of overlap between jobs for better compression
- **Thread-Safe Operations**: 
  - Fixed race conditions in parameter setting
  - Generation-tracked dictionary management preventing ABA problems
  - Proper context pool lifecycle management
- **Optimized Buffer Pool**: Automatic buffer reuse to minimize allocations
- **Frame Inspection**: Analyze compressed data without decompression
- **Sequence Producer API**: Advanced compression with custom sequence generation

### Platform Support
- **Go versions**: 1.10+ (tested with 1.24)
- **Operating Systems**: Linux, macOS, Windows, FreeBSD, illumos
- **Architectures**: amd64, arm, arm64, ppc64le, riscv64
- **Special builds**: musl libc support for Alpine Linux

## Installation

```bash
go get -u github.com/GrigoryEvko/gozstd
```

**Note**: This package requires CGO. Ensure you have a C compiler installed.

## Quick Start

### Simple Compression
```go
package main

import "github.com/GrigoryEvko/gozstd"

func main() {
    data := []byte("Hello, World!")
    
    // Compress with default settings
    compressed := gozstd.Compress(nil, data)
    
    // Decompress
    decompressed, err := gozstd.Decompress(nil, compressed)
    if err != nil {
        panic(err)
    }
}
```

### Stream Compression
```go
package main

import (
    "bytes"
    "github.com/GrigoryEvko/gozstd"
)

func main() {
    var buf bytes.Buffer
    
    // Create a writer with custom compression level
    writer := gozstd.NewWriterLevel(&buf, 5)
    defer writer.Release()
    
    // Write data
    _, err := writer.Write([]byte("Hello, streaming world!"))
    if err != nil {
        panic(err)
    }
    
    // Important: Close to flush final block
    if err := writer.Close(); err != nil {
        panic(err)
    }
}
```

## Advanced Usage

### Multi-threaded Compression
```go
package main

import (
    "bytes"
    "github.com/GrigoryEvko/gozstd"
)

func main() {
    var buf bytes.Buffer
    
    // Configure multi-threading parameters
    params := &gozstd.WriterParams{
        CompressionLevel: 5,
        NbWorkers:       4,           // Use 4 worker threads
        JobSize:         256 * 1024,  // 256KB per job
        OverlapLog:      3,           // Overlap between jobs
    }
    
    writer := gozstd.NewWriterParams(&buf, params)
    defer writer.Release()
    
    // The global thread pool is automatically used
    largeData := make([]byte, 10*1024*1024) // 10MB
    _, err := writer.Write(largeData)
    if err != nil {
        panic(err)
    }
    
    writer.Close()
}
```

### Dictionary Compression
```go
package main

import (
    "github.com/GrigoryEvko/gozstd"
)

func main() {
    // Build dictionary from samples
    samples := [][]byte{
        []byte("sample data 1"),
        []byte("sample data 2"),
        []byte("sample data 3"),
    }
    
    dictData := gozstd.BuildDict(samples, 1024) // 1KB dictionary
    
    // Create compression dictionary
    cdict, err := gozstd.NewCDict(dictData)
    if err != nil {
        panic(err)
    }
    defer cdict.Release()
    
    // Compress with dictionary
    compressed := gozstd.CompressDict(nil, []byte("data to compress"), cdict)
    
    // Create decompression dictionary
    ddict, err := gozstd.NewDDict(dictData)
    if err != nil {
        panic(err)
    }
    defer ddict.Release()
    
    // Decompress with dictionary
    decompressed, err := gozstd.DecompressDict(nil, compressed, ddict)
    if err != nil {
        panic(err)
    }
}
```

### Buffer Pool Usage
```go
package main

import (
    "github.com/GrigoryEvko/gozstd"
)

func main() {
    // Get a buffer from the pool
    buf := gozstd.GetCompressBuffer()
    defer gozstd.PutCompressBuffer(buf)
    
    // Use buffer for compression
    compressed := gozstd.Compress(buf, []byte("data"))
    
    // Buffer is automatically optimized and returned to pool
}
```

### Frame Inspection
```go
package main

import (
    "fmt"
    "github.com/GrigoryEvko/gozstd"
)

func main() {
    compressed := gozstd.Compress(nil, []byte("test data"))
    
    // Get frame information without decompressing
    info, err := gozstd.GetFrameContentSize(compressed)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Original size: %d bytes\n", info)
}
```

## Performance

### Benchmark Results
The library demonstrates excellent performance characteristics, especially with multi-threading enabled:

```
BenchmarkCompress/level-1-single-8         1000    1.05ms   952MB/s
BenchmarkCompress/level-1-multi-8          5000    0.21ms  4761MB/s
BenchmarkCompress/level-5-single-8          500    2.85ms   351MB/s
BenchmarkCompress/level-5-multi-8          2000    0.75ms  1333MB/s
BenchmarkCompress/level-9-single-8          200    5.43ms   184MB/s
BenchmarkCompress/level-9-multi-8          1000    1.82ms   549MB/s
```

### Memory Efficiency
- Zero-allocation APIs for hot paths
- Automatic buffer pooling reduces GC pressure
- Optimized buffer sizing minimizes memory waste
- Thread pool sharing reduces resource consumption

## API Documentation

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/GrigoryEvko/gozstd).

### Key Types
- `CCtx` / `DCtx`: Compression/decompression contexts with full parameter control
- `Writer` / `Reader`: io.Writer/io.Reader implementations for streaming
- `CDict` / `DDict`: Compression/decompression dictionaries
- `WriterParams` / `ReaderParams`: Advanced parameter configuration

### Key Functions
- `Compress` / `Decompress`: Simple one-shot operations
- `CompressLevel` / `DecompressLevel`: With compression level control
- `CompressDict` / `DecompressDict`: Dictionary-based operations
- `StreamCompress` / `StreamDecompress`: Streaming operations
- `BuildDict`: Create dictionaries from samples

## File Organization

The project follows a clear naming convention for better organization:

### Core Library Files
- `gozstd.go` - Main compression/decompression API
- `reader.go` / `writer.go` - Streaming implementations
- `dict.go` - Dictionary management
- `buffer_pool.go` - Buffer pool implementation
- `threadpool.go` - Global thread pool management
- Platform-specific files: `linux_amd64.go`, `darwin_arm64.go`, etc.

### Test Files
- `*_test.go` - Unit tests for corresponding source files
- `bench_*_test.go` - Benchmark tests
- `integration_*_test.go` - Integration and corpus tests
- `fuzz_*_test.go` - Fuzzing tests for robustness
- `debug_*_test.go` - Debug and diagnostic tests
- `*_example_test.go` - Example code for documentation

### Other Directories
- `cmd/examples/` - Example programs
- `cgo/` - C headers and precompiled libraries
- `contrib/` - Vendored zstd source code
- `scripts/` - Test and build scripts

## Testing

### Run All Tests
```bash
# Run with appropriate timeout for corpus tests
go test -v -timeout=600s
```

### Run Specific Test Categories
```bash
# Unit tests only
go test -v -short

# Benchmarks
go test -bench=. -benchmem

# Fuzzing tests
go test -v -run="^Fuzz"

# Integration tests
go test -v -run="^TestIntegration"
```

### Run Examples
```bash
# Simple compression example
go run cmd/examples/simple/main.go

# Advanced multi-threading example
go run cmd/examples/advanced/main.go

# Dictionary example
go run cmd/examples/dictionary/main.go
```

## FAQ

### Which Go version is required?
Go 1.10 or newer is supported, with Go 1.24+ recommended for best performance.

### How do I cross-compile?
Enable CGO and use an appropriate cross-compiler:
```bash
env CC=arm-linux-gnueabi-gcc GOOS=linux GOARCH=arm CGO_ENABLED=1 go build
```

### How do I rebuild the static libraries?
```bash
make clean libzstd.a
```

With custom flags:
```bash
MOREFLAGS=-fPIC make clean libzstd.a
```

### Is it thread-safe?
Yes! Version 1.25.0 includes comprehensive thread-safety fixes and uses a global thread pool for optimal resource sharing.

### Why are binary files included in the repo?
This simplifies installation via `go get` without requiring users to build C libraries. The libraries are regularly updated and tested across all supported platforms.

### How does the automatic thread pool work?
The library automatically creates and manages a global thread pool that's shared across all compression contexts. This improves performance and reduces resource usage compared to creating separate thread pools.

## Contributing

Contributions are welcome! Please ensure:

1. All tests pass: `go test -v -timeout=600s`
2. Code follows Go conventions: `go fmt ./...`
3. New features include tests and documentation
4. Benchmarks show no performance regression

For missing upstream zstd features, please open an issue first to discuss the implementation approach.

## Changelog

### v1.25.0 (Latest)
- **Thread Safety Improvements**:
  - Fixed critical race condition in `CCtx.SetParameter`
  - Resolved dictionary ABA problem with generation tracking
  - Fixed context pool lifecycle management
- **Multi-threading Enhancements**:
  - Added `JobSize` and `OverlapLog` parameters
  - Implemented automatic global thread pool
  - Improved parallel compression performance
- **Testing & Quality**:
  - Added comprehensive fuzzing tests
  - Created stress tests for thread safety
  - Improved test file organization with clear naming
- **Code Organization**:
  - Reorganized test files with systematic prefixes
  - Cleaned up unnecessary files
  - Better structured codebase

### Previous Releases
See [Releases](https://github.com/GrigoryEvko/gozstd/releases) for full history.

## License

MIT License. See [LICENSE](LICENSE) file for details.

## Acknowledgments

- Facebook for creating the excellent [Zstandard](https://github.com/facebook/zstd) compression algorithm
- The original [gozstd](https://github.com/valyala/gozstd) author for the initial implementation
- All contributors who have helped improve this library
---

For bug reports and feature requests, please use the [GitHub Issues](https://github.com/GrigoryEvko/gozstd/issues) page.

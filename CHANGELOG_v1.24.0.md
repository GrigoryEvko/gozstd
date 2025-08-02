# gozstd v1.24.0 Release Notes

This release integrates multiple community pull requests that were pending in the original valyala/gozstd repository, along with comprehensive testing improvements.

## Major Improvements

### 1. Advanced Compression API (PR #25)
- Added `CCtx` type for advanced compression contexts
- Implemented parameter setting with `SetParameter()` method
- Support for all ZSTD compression parameters
- Added checksum support with validation
- Created comprehensive `cparams.go` with all compression parameters

### 2. CGO Wrapper Performance Improvements (PR #49)
- Replaced `uintptr_t` with `void*` in C wrappers
- Direct Go slice usage via reflect.SliceHeader
- 5-7% performance improvement for large buffers
- Cleaner and safer CGO implementation

### 3. Public CompressDictLevel API (PR #63)
- Made `CompressDictLevel` function public
- Allows better control over dictionary compression levels
- Useful for applications requiring fine-tuned compression

### 4. Memory-Optimized Dictionary Functions (PR #60)
- Added `NewCDictByRef` and `NewDDictByRef` functions
- Avoids copying dictionary data (uses `*_byReference` C functions)
- Significant memory savings for large dictionaries
- Added corresponding test coverage

### 5. RISC-V Architecture Support (PR #66)
- Added RISC-V 64-bit architecture support
- Updated build system for riscv64 target
- Modernized Zig builder infrastructure

## Infrastructure Improvements

### 6. Modern Docker Build Environment
- Created new Dockerfile with Alpine Linux base
- Latest Zig compiler from Alpine packages
- Replaced outdated euantorano/zig:0.10.1 image
- Fixed ARM64 cross-compilation issues

### 7. Comprehensive Testing Suite
- **Corpus Tests**: Added Silesia Corpus compression tests
- **Benchmarks**: Created timing benchmarks comparing raw vs wrapper
- **Basic Fuzz Tests**: 10 fuzz tests for core functionality
- **Aggressive Fuzz Tests**: 33 specialized fuzz tests targeting:
  - Memory safety vulnerabilities
  - Parameter boundary violations
  - State machine abuse
  - Corrupted data handling
  - Race conditions
  - Resource exhaustion

### 8. Build System Fixes
- Fixed Makefile clean target issues
- Resolved RM command problems with special filenames
- Updated minimum Go version to 1.24

## Bug Discoveries

The aggressive fuzzing revealed:
1. ZSTD can sometimes decompress data with multiple bit flips
2. Race condition when sharing CCtx between goroutines (causes crashes)

## Performance

- CGO wrapper improvements: 5-7% faster for large buffers
- Compression ratios: Identical to raw ZSTD
- Overall wrapper performance: ~10% faster than raw C API calls

## Breaking Changes

- Minimum Go version updated from 1.12 to 1.24

## Usage Examples

### Advanced Compression API
```go
ctx := gozstd.NewCCtx()
ctx.SetParameter(gozstd.ZSTD_c_compressionLevel, 5)
ctx.SetParameter(gozstd.ZSTD_c_checksumFlag, 1)
compressed, err := ctx.Compress(nil, data)
```

### Memory-Optimized Dictionaries
```go
// Avoid copying dictionary data
dict := getLargeDictionary() // []byte
cd, err := gozstd.NewCDictByRef(dict)
defer cd.Release()
compressed := gozstd.CompressDict(nil, data, cd)
```

## Contributors

- GrigoryEvko: Integration, testing, and release management
- Original PR authors from valyala/gozstd community

## Next Steps

- Investigate and fix the race condition in shared CCtx usage
- Consider adding mutex protection for CCtx concurrent access
- Further performance optimizations
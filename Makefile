GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOOS_GOARCH := $(GOOS)_$(GOARCH)
GOOS_GOARCH_NATIVE := $(shell go env GOHOSTOS)_$(shell go env GOHOSTARCH)
LIBZSTD_NAME := libzstd_$(GOOS_GOARCH).a
ZSTD_VERSION ?= v1.5.7-kernel
ZIG_BUILDER_IMAGE := euantorano/zig:0.10.1

# Detect available container runtime
CONTAINER_RUNTIME := $(shell \
	if command -v docker >/dev/null 2>&1; then \
		echo "docker"; \
	elif command -v nerdctl >/dev/null 2>&1; then \
		echo "nerdctl"; \
	elif command -v podman >/dev/null 2>&1; then \
		echo "podman"; \
	else \
		echo "none"; \
	fi)

# Show which runtime is being used
$(info Using container runtime: $(CONTAINER_RUNTIME))

# Parallel compilation flags
JOBS := $(shell nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
MAKEFLAGS += -j$(JOBS)

.PHONY: libzstd.a $(LIBZSTD_NAME) test test-no-lto bench bench-no-lto

libzstd.a: $(LIBZSTD_NAME)
$(LIBZSTD_NAME):
ifeq ($(GOOS_GOARCH),$(GOOS_GOARCH_NATIVE))
	rm -f $(LIBZSTD_NAME)
	cd zstd/lib && ZSTD_LEGACY_SUPPORT=0 AR="gcc-ar" ARFLAGS="rcs" MOREFLAGS="-DZSTD_MULTITHREAD=1 -O3 -flto=auto $(MOREFLAGS)" LDFLAGS="-flto=auto -fuse-linker-plugin" $(MAKE) clean libzstd.a
	mv zstd/lib/libzstd.a $(LIBZSTD_NAME)
else ifeq ($(GOOS_GOARCH),linux_amd64)
	TARGET=x86_64-linux GOARCH=amd64 GOOS=linux ARCH_FLAGS="-mcpu=x86_64+sse4_2+avx2+bmi2" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),linux_arm)
	TARGET=arm-linux-gnueabi GOARCH=arm GOOS=linux ARCH_FLAGS="-mcpu=generic" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),linux_arm64)
	TARGET=aarch64-linux GOARCH=arm64 GOOS=linux ARCH_FLAGS="-mcpu=generic" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),linux_ppc64le)
	TARGET=x86_64-linux GOARCH=ppc64le GOOS=linux ARCH_FLAGS="" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),linux_musl_amd64)
	TARGET=x86_64-linux-musl GOARCH=amd64 GOOS=linux_musl ARCH_FLAGS="-mcpu=x86_64+sse4_2+avx2+bmi2" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),linux_musl_arm64)
	TARGET=aarch64-linux-musl GOARCH=arm64 GOOS=linux_musl ARCH_FLAGS="-mcpu=generic" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),darwin_arm64)
	TARGET=aarch64-macos GOARCH=arm64 GOOS=darwin ARCH_FLAGS="-mcpu=apple_m1" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),darwin_amd64)
	TARGET=x86_64-macos GOARCH=amd64 GOOS=darwin ARCH_FLAGS="-mcpu=x86_64+sse4_2+avx2" $(MAKE) package-arch
else ifeq ($(GOOS_GOARCH),windows_amd64)
	TARGET=x86_64-windows GOARCH=amd64 GOOS=windows GOARCH=amd64 ARCH_FLAGS="-mcpu=x86_64+sse4_2+avx2+bmi2" $(MAKE) package-arch
endif

package-arch:
ifeq ($(CONTAINER_RUNTIME),none)
	$(error No container runtime found. Please install docker, nerdctl, or podman for cross-compilation)
endif
	rm -f $(LIBZSTD_NAME)
	$(CONTAINER_RUNTIME) run --rm \
		--entrypoint /bin/sh \
		--mount type=bind,src="$(shell pwd)",dst=/zstd \
		-w /zstd/zstd/lib \
		$(DOCKER_OPTS) \
		$(ZIG_BUILDER_IMAGE) \
		-c 'apk add --no-cache make && \
			if echo "$(TARGET)" | grep -q "macos\|darwin"; then \
				LTO_FLAG=""; \
			else \
				LTO_FLAG="-flto=auto"; \
			fi; \
			ZSTD_LEGACY_SUPPORT=0 AR="zig ar" \
			CC="zig cc -target $(TARGET) -O3 $$LTO_FLAG $(ARCH_FLAGS)" \
			CXX="zig cc -target $(TARGET) -O3 $$LTO_FLAG $(ARCH_FLAGS)" \
			MOREFLAGS="-DZSTD_MULTITHREAD=1 -O3 $$LTO_FLAG $(ARCH_FLAGS) $(MOREFLAGS)" \
			make -j$(shell nproc 2>/dev/null || echo 4) clean libzstd.a'
	mv -f zstd/lib/libzstd.a $(LIBZSTD_NAME)

# freebsd and illumos aren't supported by zig compiler atm.
release:
	GOOS=linux GOARCH=amd64 $(MAKE) libzstd.a
	GOOS=linux GOARCH=arm64 $(MAKE) libzstd.a
	GOOS=linux GOARCH=arm $(MAKE) libzstd.a
	GOOS=linux GOARCH=ppc64le $(MAKE) libzstd.a
	GOOS=linux_musl GOARCH=amd64 $(MAKE) libzstd.a
	GOOS=linux_musl GOARCH=arm64 $(MAKE) libzstd.a
	GOOS=darwin GOARCH=arm64 $(MAKE) libzstd.a
	GOOS=darwin GOARCH=amd64 $(MAKE) libzstd.a
	GOOS=windows GOARCH=amd64 $(MAKE) libzstd.a

clean:
	rm -f $(LIBZSTD_NAME)
	cd zstd && $(MAKE) clean

update-zstd:
	rm -rf zstd-tmp
	git clone --branch $(ZSTD_VERSION) --depth 1 https://github.com/Facebook/zstd zstd-tmp
	rm -rf zstd-tmp/.git
	rm -rf zstd
	mv zstd-tmp zstd
	cp zstd/lib/zstd.h .
	cp zstd/lib/zdict.h .
	cp zstd/lib/zstd_errors.h .
	$(MAKE) release

test:
	CGO_LDFLAGS_ALLOW='-flto.*' CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go test -ldflags="-linkmode=external" -v

test-no-lto:
	CGO_ENABLED=1 GOEXPERIMENT=cgocheck2 go test -v

bench:
	CGO_LDFLAGS_ALLOW='-flto.*' CGO_ENABLED=1 go test -ldflags="-linkmode=external" -bench=.

bench-no-lto:
	CGO_ENABLED=1 go test -bench=.

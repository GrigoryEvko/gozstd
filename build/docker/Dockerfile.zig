# Modern Zig builder with latest Alpine Linux and fresh Zig installation
FROM alpine:latest

# Install build essentials and latest Zig
RUN apk add --no-cache \
    zig \
    make \
    gcc \
    musl-dev \
    linux-headers \
    git \
    bash \
    coreutils

# Set working directory
WORKDIR /build

# Verify Zig installation and show version
RUN zig version

# Set default shell to bash for better compatibility
SHELL ["/bin/bash", "-c"]

# Add labels for better container management
LABEL maintainer="GrigoryEvko" \
      description="Modern Zig builder with latest Alpine Linux" \
      version="latest" \
      zig.version="latest-from-alpine"

# Default command
CMD ["/bin/bash"]
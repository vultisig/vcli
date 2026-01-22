# Build stage - use Debian for glibc compatibility with go-wrappers
FROM golang:1.25-bookworm AS builder

ARG TARGETARCH=amd64

RUN apt-get update && apt-get install -y git build-essential && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Remove only local path replace directives (keep version fixes)
RUN sed -i '/=> *\.\./d' go.mod

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/

# Build binary with CGO enabled for crypto libs
RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH} go build -o vcli ./cmd/vcli

# Find and copy shared libraries (from includes/linux/ subdirectory)
RUN mkdir -p /build/libs && \
    find /go/pkg/mod/github.com/vultisig/go-wrappers*/includes/linux -name "*.so" -exec cp {} /build/libs/ \;

# Runtime stage - use Debian slim for glibc compatibility
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy shared libraries
COPY --from=builder /build/libs/*.so /usr/lib/

# Copy binary from builder
COPY --from=builder /build/vcli /usr/local/bin/vcli

# Create home directory for config storage
RUN mkdir -p /root/.vultisig

ENTRYPOINT ["vcli"]

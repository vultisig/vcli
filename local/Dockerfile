# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Remove local replace directives for Docker build (use remote versions)
RUN sed -i '/^replace/,/^)/d' go.mod && \
    sed -i '/replace.*=>.*\.\.\//d' go.mod

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o vcli ./cmd/vcli

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/vcli /usr/local/bin/vcli

# Create home directory for config storage
RUN mkdir -p /root/.vultisig

ENTRYPOINT ["vcli"]

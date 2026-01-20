FROM golang:1.25.5 AS builder

ARG TARGETARCH

RUN apt-get update && apt-get install -y clang lld wget

RUN wget https://github.com/vultisig/go-wrappers/archive/refs/heads/master.tar.gz && \
    tar -xzf master.tar.gz && \
    cd go-wrappers-master && \
    mkdir -p /usr/local/lib/dkls/includes && \
    cp includes/go-dkls.h includes/go-schnorr.h /usr/local/lib/dkls/includes/ && \
    cp -r includes/linux-${TARGETARCH} /usr/local/lib/dkls/includes/linux

WORKDIR /app
COPY local/go.mod local/go.sum ./
RUN go mod download
COPY local/ .

ENV CGO_ENABLED=1
ENV CC=clang
ENV CGO_LDFLAGS=-fuse-ld=lld
ENV LD_LIBRARY_PATH=/usr/local/lib/dkls/includes/linux:$LD_LIBRARY_PATH
RUN go build -o vcli cmd/vcli/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/vcli /usr/local/bin/vcli
COPY --from=builder /usr/local/lib/dkls /usr/local/lib/dkls
ENV LD_LIBRARY_PATH=/usr/local/lib/dkls/includes/linux:$LD_LIBRARY_PATH

ENTRYPOINT ["sleep", "infinity"]

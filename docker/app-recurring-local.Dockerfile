# Build app-recurring services with local verifier dependency
# Must be built from parent directory containing both app-recurring and verifier:
# docker build -f vcli/docker/app-recurring-local.Dockerfile --build-arg BINARY=tx_indexer -t app-recurring-txindexer:local .

# Stage 1: Download go-wrappers (cached layer - rarely changes)
FROM golang:1.25-bookworm AS dkls-setup

RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

RUN wget -q https://github.com/vultisig/go-wrappers/archive/refs/heads/master.tar.gz && \
    tar -xzf master.tar.gz && \
    mkdir -p /usr/local/lib/dkls && \
    cp -r go-wrappers-master/includes /usr/local/lib/dkls/ && \
    rm -rf master.tar.gz go-wrappers-master

# Stage 2: Build (with local verifier dependency)
FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y clang && rm -rf /var/lib/apt/lists/*

COPY --from=dkls-setup /usr/local/lib/dkls /usr/local/lib/dkls

ARG BINARY=server

WORKDIR /build

# Copy both repositories
COPY verifier ./verifier
COPY app-recurring ./app-recurring

WORKDIR /build/app-recurring

ENV CGO_ENABLED=1
ENV CC=clang
ENV LD_LIBRARY_PATH=/usr/local/lib/dkls/includes/linux/

RUN go build -o /app/${BINARY} ./cmd/${BINARY}

# Stage 3: Runtime (minimal image)
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/lib/dkls/includes/linux/*.so /usr/local/lib/
ARG BINARY=server
COPY --from=builder /app/${BINARY} /app/${BINARY}

RUN ldconfig

WORKDIR /app
EXPOSE 8080 8088

CMD ["/app/main"]

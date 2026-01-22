# Stage 1: Download go-wrappers (cached layer - rarely changes)
FROM golang:1.25-bookworm AS dkls-setup

RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

# This layer is cached unless go-wrappers repo changes
RUN wget -q https://github.com/vultisig/go-wrappers/archive/refs/heads/master.tar.gz && \
    tar -xzf master.tar.gz && \
    mkdir -p /usr/local/lib/dkls && \
    cp -r go-wrappers-master/includes /usr/local/lib/dkls/ && \
    rm -rf master.tar.gz go-wrappers-master

# Stage 2: Download Go dependencies (cached layer - changes when go.mod changes)
FROM golang:1.25-bookworm AS deps

RUN apt-get update && apt-get install -y clang && rm -rf /var/lib/apt/lists/*

# Copy pre-downloaded go-wrappers from cache stage
COPY --from=dkls-setup /usr/local/lib/dkls /usr/local/lib/dkls

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

# Stage 3: Build (rebuilds only when source changes)
FROM deps AS builder

ARG BINARY=verifier

COPY . .

ENV CGO_ENABLED=1
ENV CC=clang
ENV LD_LIBRARY_PATH=/usr/local/lib/dkls/includes/linux/

RUN go build -o /app/main ./cmd/${BINARY}

# Stage 4: Runtime (minimal image)
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/lib/dkls/includes/linux/*.so /usr/local/lib/
COPY --from=builder /app/main /app/main

RUN ldconfig

WORKDIR /app
EXPOSE 8080 8088

CMD ["/app/main"]

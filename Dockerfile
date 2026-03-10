# --- Stage 1: Build stage ---
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source tree
COPY . .

# Compile all binaries for ARM64 architecture (Apple Silicon native)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/arbiter ./cmd/arbiter
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/scheduler ./cmd/scheduler
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/poller ./cmd/poller
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/reactionner ./cmd/reactionner
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/broker ./cmd/broker

# --- Stage 2: Runtime stage ---
FROM alpine:latest

# Install runtime dependencies (adding bash for your convenience)
RUN apk add --no-cache ca-certificates tzdata bash

WORKDIR /shinsakuto

# Copy compiled binaries from the builder stage
COPY --from=builder /app/bin/* ./bin/

# Create the directory structure
RUN mkdir -p /shinsakuto/etc/shinsakuto/conf.d/standalone \
             /shinsakuto/etc/shinsakuto/standalone \
             /shinsakuto/var/log

# Copy configuration files from your local etc/ directory to the image
# This assumes your local folder structure matches the internal one
COPY etc/shinsakuto/ /shinsakuto/etc/shinsakuto/

# Create the entrypoint script
RUN echo '#!/bin/sh' > /shinsakuto/entrypoint.sh && \
    echo './bin/arbiter -config /shinsakuto/etc/arbiter.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/scheduler  -config /shinsakuto/etc/scheduler.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/reactionner  -config /shinsakuto/etc/reactionner.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/broker -config /shinsakuto/etc/broker.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/poller -config /shinsakuto/etc/poller.json' >> /shinsakuto/entrypoint.sh && \
    chmod +x /shinsakuto/entrypoint.sh

# Expose internal ports
EXPOSE 8080 8081 8083 8084

ENTRYPOINT ["/shinsakuto/entrypoint.sh"]

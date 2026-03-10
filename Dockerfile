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

# Compile binaries for the target architecture
# We use standard go build; CGO_ENABLED=0 ensures portability on Alpine
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/arbiter ./cmd/arbiter
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/scheduler ./cmd/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/poller ./cmd/poller
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/reactionner ./cmd/reactionner
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/broker ./cmd/broker

# --- Stage 2: Runtime stage ---
FROM alpine:latest

# Install runtime dependencies and 'jq' to potentially parse JSON in scripts
RUN apk add --no-cache ca-certificates tzdata bash curl iputils

WORKDIR /shinsakuto

# Copy compiled binaries from the builder stage
COPY --from=builder /app/bin/* ./bin/

# Create necessary directory structure for standalone persistence and logs
RUN mkdir -p /shinsakuto/var/log \
             /shinsakuto/var/lib/scheduler \
             /shinsakuto/etc/shinsakuto/conf.d/standalone

# Copy ONLY the standalone configuration files to the root etc for the entrypoint
#COPY etc/shinsakuto/standalone/*.json /shinsakuto/etc/
# Copy the monitoring definitions (Hosts, Services, Commands)
#COPY etc/shinsakuto/conf.d/standalone/ /shinsakuto/etc/shinsakuto/conf.d/standalone/

# Create the standalone entrypoint script
# It starts all components in the background and keeps the poller in the foreground
RUN echo '#!/bin/sh' > /shinsakuto/entrypoint.sh && \
    echo './bin/broker -c /shinsakuto/etc/broker.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/reactionner -c /shinsakuto/etc/reactionner.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/scheduler -c /shinsakuto/etc/scheduler.json &' >> /shinsakuto/entrypoint.sh && \
    echo './bin/arbiter -c /shinsakuto/etc/arbiter.json &' >> /shinsakuto/entrypoint.sh && \
    echo 'sleep 2' >> /shinsakuto/entrypoint.sh && \
    echo './bin/poller -c /shinsakuto/etc/poller.json' >> /shinsakuto/entrypoint.sh && \
    chmod +x /shinsakuto/entrypoint.sh

# Expose ports for all components in standalone mode
# Arbiter: 8080, Scheduler: 8090, Reactionner: 8070, Broker: 8084
EXPOSE 8080 8090 8070 8084

ENTRYPOINT ["/shinsakuto/entrypoint.sh"]

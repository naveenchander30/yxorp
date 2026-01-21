# Build Stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o yxorp-waf ./cmd/waf

# Final Stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl && \
    addgroup -S yxorp && \
    adduser -S yxorp -G yxorp

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/yxorp-waf .

# Create directories
RUN mkdir -p /app/configs /app/logs && \
    chown -R yxorp:yxorp /app

# Copy default config (will be overridden by volume mount)
COPY configs/rules.yaml /app/configs/

# Switch to non-root user
USER yxorp

# Expose ports
EXPOSE 8080 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8081/metrics || exit 1

# Run the binary
CMD ["./yxorp-waf"]

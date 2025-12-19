# Build Stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o yxorp ./cmd/waf

# Final Stage
FROM alpine:latest

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/yxorp .

# Copy configuration
COPY --from=builder /app/configs ./configs

# Expose ports (App + Metrics)
EXPOSE 8080 8081

# Run the binary
CMD ["./yxorp"]

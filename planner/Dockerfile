# Planner Service Dockerfile
# Multi-stage build for Go service

ARG GO_VERSION=1.25

# Build stage
FROM golang:${GO_VERSION}-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY contracts/ contracts/
COPY planner/go.mod planner/go.sum ./

# Download dependencies
RUN cd planner && go mod download

# Copy source code
COPY planner/main.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=v1.0.0 -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -a -installsuffix cgo \
    -o planner \
    main.go

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/planner /app/planner

# Expose gRPC port
EXPOSE 50080

# Run as non-root user (distroless default: 65532)
USER nonroot:nonroot

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/app/planner", "-health-check"] || exit 1

# Run the service
ENTRYPOINT ["/app/planner"]



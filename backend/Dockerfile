# =============================================================================
# Chaos-Sec Backend - Multi-Stage Docker Build
# =============================================================================
# Stage 1: Build the Go binary
# Stage 2: Minimal runtime image
# =============================================================================

# ---------------------------------------------------------------------------
# Stage 1: Builder
# ---------------------------------------------------------------------------
FROM golang:1.25-alpine AS builder

# Install build dependencies
# ca-certificates for TLS, git for go modules, tzdata for timezone support
RUN apk add --no-cache ca-certificates git tzdata

# Set the working directory inside the container
WORKDIR /build

# Set environment variables for static Go binary build
# CGO_ENABLED=0  - disable cgo for static linking
# GOOS=linux     - target Linux
# GOARCH=amd64   - target architecture
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GO111MODULE=on

# Copy go.mod and go.sum first to leverage Docker layer caching
# Dependencies are only re-downloaded when go.mod or go.sum change
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy the entire source code
COPY . .

# Build the binary with optimizations
# -ldflags="-w -s" strips debug info and symbol table for smaller binary
# -trimpath removes source directory paths from the binary
RUN go build \
    -ldflags="-w -s" \
    -trimpath \
    -o /build/chaos-sec-backend \
    ./cmd/backend

# Verify the binary was built
RUN ls -la /build/chaos-sec-backend && \
    /build/chaos-sec-backend --version 2>/dev/null || true

# ---------------------------------------------------------------------------
# Stage 2: Runtime
# ---------------------------------------------------------------------------
FROM alpine:3.18 AS runtime

# Install runtime dependencies
# ca-certificates for TLS connections to external services
# tzdata for timezone support
# curl for healthcheck
RUN apk add --no-cache ca-certificates tzdata curl && \
    cp /usr/share/zoneinfo/UTC /etc/localtime && \
    echo "UTC" > /etc/timezone && \
    apk del tzdata

# Create a non-root user and group for security
# -D = create a system user, -H = don't create home directory
# -g '' = no password
RUN addgroup -S -g 1000 appgroup && \
    adduser -S -u 1000 -G appgroup -H -D -s /sbin/nologin appuser

# Set the working directory
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /build/chaos-sec-backend /app/chaos-sec-backend

# Copy any migration files needed at runtime
COPY --from=builder /build/migrations /app/migrations

# Set ownership to the non-root user
RUN chown -R appuser:appgroup /app

# Switch to the non-root user
USER appuser

# Expose the application port (API) and metrics port
EXPOSE 8080 9090

# Define environment variables with sensible defaults
ENV APP_ENV=production \
    SERVER_PORT=8080 \
    METRICS_PORT=9090 \
    LOG_LEVEL=info \
    LOG_FORMAT=json

# Health check against the /health endpoint
# --interval=30s: check every 30 seconds
# --timeout=5s: allow 5 seconds for the check to complete
# --start-period=10s: give the app 10s to start before checking
# --retries=3: fail after 3 consecutive failures
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health/live || exit 1

# Set the entrypoint
ENTRYPOINT ["/app/chaos-sec-backend"]

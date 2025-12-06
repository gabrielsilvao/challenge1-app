# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git for fetching dependencies
RUN apk add --no-cache git ca-certificates

# Copy all source code first
COPY . .

# Download and resolve dependencies (generates go.sum)
RUN go mod tidy && go mod download

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o main .

# Final stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .

# Copy templates
COPY --from=builder /app/templates ./templates

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

# Expose port
EXPOSE 8080

# Environment variables for OpenTelemetry (can be overridden)
ENV OTEL_SERVICE_NAME=sample-web-app \
    OTEL_SERVICE_VERSION=1.0.0 \
    OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318 \
    OTEL_INSECURE=true \
    ENV=production

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./main"]

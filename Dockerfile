# Build stage
FROM golang:1.25.1-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o nodeport-router ./main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS/TLS connections (needed for Kubernetes API)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/nodeport-router .

# Run as non-root user
RUN addgroup -g 1000 nodeport-router && \
    adduser -D -u 1000 -G nodeport-router nodeport-router && \
    chown -R nodeport-router:nodeport-router /app

USER nodeport-router

# The binary will use environment variables for configuration
# and expects to run in a Kubernetes cluster or with kubeconfig
ENTRYPOINT ["./nodeport-router"]


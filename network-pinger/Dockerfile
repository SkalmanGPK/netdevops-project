FROM golang:1.26-alpine AS builder

# Install git for certificate management
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code (including env.go/loadEnv)
COPY . .

# Compile binary statically to ensure it works in a minimal image
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mesh-pinger .

# === stage 2: minimal runtime image ===
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

# Create a safe none-root-user
RUN adduser -D -u 10001 appuser
USER appuser

WORKDIR /home/appuser
# Copy only the compiled binary from the builder stage
COPY --from=builder /app/mesh-pinger .


EXPOSE 8090

# Start the application
ENTRYPOINT ["/mesh-pinger"]

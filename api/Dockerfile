############################################
# 1. BUILD STAGE
############################################
FROM golang:1.22-alpine AS builder

# Enable faster builds
RUN apk add --no-cache git build-base

WORKDIR /app

# Load dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy full source
COPY . .

# Build the Go binary
RUN go build -o server .


############################################
# 2. RUN STAGE
############################################
FROM alpine:3.19

# Install certificates (needed for HTTPS, Postgres SSL, AWS SDK)
RUN apk add --no-cache ca-certificates

WORKDIR /root/

# Copy the built binary
COPY --from=builder /app/server .

# Ensure the binary is executable
RUN chmod +x ./server

# Run the server
CMD ["./server"]

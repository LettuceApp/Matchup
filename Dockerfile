############################################
# 1. BUILD STAGE
############################################
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git build-base

WORKDIR /app

# Copy go.mod and go.sum first
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire repository
COPY . .

# Build Go application from root (main.go)
RUN go build -o server .

############################################
# 2. RUN STAGE
############################################
FROM alpine:3.19
RUN apk add --no-cache ca-certificates

WORKDIR /root/

COPY --from=builder /app/server .

RUN chmod +x server

CMD ["./server"]

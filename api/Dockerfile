############################################
# 1. BUILD STAGE
############################################
FROM golang:1.24-alpine AS builder

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

RUN apk add --no-cache ca-certificates

WORKDIR /root/

COPY --from=builder /app/server .

RUN chmod +x ./server

CMD ["./server"]

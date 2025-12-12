############################################
# 1. BUILD STAGE
############################################
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git build-base

# Set working directory INSIDE the api folder
WORKDIR /app

# Copy ONLY the api folder
COPY ./api/go.mod ./api/go.sum ./
RUN go mod download

COPY ./api .

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

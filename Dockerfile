# ---- BUILD STAGE ----
    FROM golang:1.21 AS builder

    WORKDIR /app
    
    # Copy go.mod and go.sum first for caching
    COPY api/go.mod api/go.sum ./api/
    
    # Download dependencies
    RUN cd api && go mod download
    
    # Copy the rest of the source code
    COPY api ./api
    COPY cmd ./cmd
    
    # Build the main application from cmd/main.go
    RUN cd cmd && go build -o /app/server main.go
    
    # ---- RUN STAGE ----
    FROM alpine:3.19
    
    RUN apk add --no-cache ca-certificates
    
    WORKDIR /root/
    
    # Copy the compiled binary
    COPY --from=builder /app/server .
    
    RUN chmod +x server
    
    EXPOSE 8080
    
    CMD ["./server"]
    
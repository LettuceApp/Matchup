# ---- BUILD STAGE ----
    FROM golang:1.24-alpine AS builder
    

    WORKDIR /app
    
    # Copy go module files
    COPY go.mod go.sum ./
    RUN go mod download
    
    # Copy the entire repository
    COPY . .
    
    # Build your Go server using cmd/main.go
    RUN go build -o server ./cmd/main.go
    
    # ---- RUN STAGE ----
    FROM alpine:3.19
    
    RUN apk add --no-cache ca-certificates
    
    WORKDIR /root/
    
    COPY --from=builder /app/server .
    
    RUN chmod +x server
    
    EXPOSE 8080
    
    CMD ["./server"]
    
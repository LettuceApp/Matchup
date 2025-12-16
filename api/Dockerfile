# -------- 1. Build Stage --------
    FROM golang:1.24 AS build

    WORKDIR /app
    
    # Copy go mod files
    COPY go.mod go.sum ./
    RUN go mod download
    
    # Copy the entire api folder (including cmd/)
    COPY . .
    
    # Build the binary from cmd/main.go
    RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd
    
    # -------- 2. Runtime Stage --------
    FROM gcr.io/distroless/base-debian12:latest
    
    WORKDIR /app
    
    COPY --from=build /app/server .
    
    # Render sets PORT automatically but we define a fallback
    ENV PORT=8888
    EXPOSE 8888
    
    ENTRYPOINT ["./server"]
    
# -------- 1. Build Stage --------
    FROM --platform=$BUILDPLATFORM golang:1.25 AS build
    ARG TARGETOS=linux
    ARG TARGETARCH=amd64

    WORKDIR /app

    # Copy go mod files
    COPY go.mod go.sum ./
    RUN go mod download

    # Copy the entire api folder (including cmd/)
    COPY . .

    # Cross-compile natively (no QEMU needed for the Go compiler)
    RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o server ./cmd

    # -------- 2. Runtime Stage --------
    FROM --platform=linux/amd64 gcr.io/distroless/base-debian12:latest
    
    WORKDIR /app
    
    COPY --from=build /app/server .
    
    # Render sets PORT automatically but we define a fallback
    ENV PORT=8888
    EXPOSE 8888
    
    ENTRYPOINT ["./server"]
    
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

    # Cross-compile natively (no QEMU needed for the Go compiler).
    # Three binaries land in the image: ./server is the HTTP API,
    # ./worker is the background-job runner, and ./cron is the
    # scheduler (replaces the former Dagster orchestration stack).
    # The K8s worker + cron deployments select which to run via the
    # container `command`.
    RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o server ./cmd \
     && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o worker ./cmd/worker \
     && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o cron ./cmd/cron

    # -------- 2. Runtime Stage --------
    FROM --platform=linux/amd64 gcr.io/distroless/base-debian12:latest

    WORKDIR /app

    COPY --from=build /app/server .
    COPY --from=build /app/worker .
    COPY --from=build /app/cron .

    # Render sets PORT automatically but we define a fallback
    ENV PORT=8888
    EXPOSE 8888

    ENTRYPOINT ["./server"]
    
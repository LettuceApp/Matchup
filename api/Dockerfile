# ================================
# 🚧 1. Build Stage
# ================================
FROM golang:1.24 AS build

# Set working directory inside the container
WORKDIR /app

# Copy only go.mod and go.sum first → enables cached builds
COPY go.mod go.sum ./

RUN go mod download

# Copy the entire backend source
COPY . .

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server .


# ================================
# 🚀 2. Runtime Stage
# ================================
FROM gcr.io/distroless/base-debian12:latest

WORKDIR /app

# Bring the compiled binary from build stage into this minimal image
COPY --from=build /app/server .

# Render sets PORT automatically, but default to 8888
ENV PORT=8888

EXPOSE 8888

# Run the binary
ENTRYPOINT ["./server"]

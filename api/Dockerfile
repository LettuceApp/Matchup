FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server ./cmd

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 8888
CMD ["/app/server"]

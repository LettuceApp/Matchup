FROM golang:1.24 as build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o server .

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=build /app/server .

EXPOSE 8080

CMD ["/app/server"]

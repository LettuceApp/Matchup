# backend /api/Dockerfile
FROM golang:1.22 AS build
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o server .

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /app/server .
EXPOSE 8888
CMD ["./server"]

FROM golang:1.22 AS build

WORKDIR /app
COPY api/go.mod api/go.sum ./
RUN go mod download

COPY api/ .
RUN go build -o server .

FROM gcr.io/distroless/base-debian12
COPY --from=build /app/server /server
CMD ["/server"]

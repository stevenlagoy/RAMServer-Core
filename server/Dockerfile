# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -o /out/ramserver ./cmd/ramserver

FROM alpine:3.20
RUN adduser -D -u 10001 ramserver
WORKDIR /app
COPY --from=build /out/ramserver /app/ramserver
USER ramserver
EXPOSE 9000
ENTRYPOINT ["/app/ramserver"]
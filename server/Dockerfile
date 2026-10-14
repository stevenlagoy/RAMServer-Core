# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
COPY --from=bufbuild/buf:1.73.0 /usr/local/bin/buf /usr/local/bin/buf
WORKDIR /src
COPY server/go.mod server/go.sum server/
RUN cd server && go mod download
# Generated protobuf code is gitignored, so generate it from the schema here
COPY buf.yaml buf.gen.yaml ./
COPY proto/ proto/
RUN buf generate
COPY server/ server/
WORKDIR /src/server
RUN CGO_ENABLED=0 go build -o /out/ramserver ./cmd/ramserver

FROM alpine:3.20
RUN adduser -D -u 10001 ramserver
WORKDIR /app
COPY --from=build /out/ramserver /app/ramserver
USER ramserver
EXPOSE 9000
ENTRYPOINT ["/app/ramserver"]

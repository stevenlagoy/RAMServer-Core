# The tic-tac-toe client in a container. It's a terminal application, so run it with -it and it draws in
# your own terminal window:
#
#   docker build -t ttt .
#   docker run --rm -it ttt --server hosted
#   docker run --rm -it ttt --server host.docker.internal:8080   # a server on your machine
#
# Building needs network access: the protocol schema is downloaded from RAMServer-Core's main branch.

# ---- Build: compile ttt and run the unit tests ----
FROM debian:bookworm-slim AS build
COPY --from=bufbuild/buf:1.73.0 /usr/local/bin/buf /usr/local/bin/buf
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential pkg-config git ca-certificates \
        protobuf-compiler protobuf-c-compiler libprotobuf-c-dev \
        libwebsockets-dev libssl-dev libcmocka-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN make BUILD=build/docker && make test BUILD=build/docker

# ---- Run: only the binary and the libraries it needs ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        libprotobuf-c1 libwebsockets17 ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home player
COPY --from=build /src/build/docker/ttt /usr/local/bin/ttt
# UTF-8, so the board's box-drawing characters display correctly
ENV LANG=C.UTF-8
USER player
ENTRYPOINT ["ttt"]

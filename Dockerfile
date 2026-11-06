# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/media-worker .

# Static ffmpeg build: a single binary instead of the distro package with its ~100 shared libraries.
FROM mwader/static-ffmpeg:7.1 AS ffmpeg

# distroless/static brings CA certificates and a writable /tmp, and runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=ffmpeg /ffmpeg /usr/local/bin/ffmpeg
COPY --from=build /out/media-worker /usr/local/bin/media-worker
ENV FFMPEG_PATH=/usr/local/bin/ffmpeg
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/media-worker"]

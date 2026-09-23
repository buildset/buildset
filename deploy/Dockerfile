# syntax=docker/dockerfile:1

# One Dockerfile for every binary, selected by CMD, so they share a Go version and a dependency
# layer.
FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG CMD
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${CMD}

# A static image, possible because the SQLite driver is pure Go. It has no shell, which is why the
# healthcheck is the binary probing its own readiness.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/service /service

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/service"]

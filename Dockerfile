# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache build-base

WORKDIR /app

COPY . .

# BuildKit cache mount persists the Go build cache across image builds (incremental recompiles).
# Dropped `-a -installsuffix cgo` — those forced a full rebuild and defeated the cache.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 CGO_CFLAGS="-D_LARGEFILE64_SOURCE" GOOS=linux \
    go build -mod=vendor -o ant ./cmd/api/main.go

FROM alpine:3.22

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/ant .
COPY --from=builder /app/config ./config

EXPOSE 8082 8083

CMD ["./ant"]

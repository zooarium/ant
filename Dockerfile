ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache build-base

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY vendor/ vendor/
COPY . .

RUN CGO_ENABLED=1 CGO_CFLAGS="-D_LARGEFILE64_SOURCE" GOOS=linux go build -mod=vendor -a -installsuffix cgo -o ant ./cmd/api/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/ant .

EXPOSE 8082

CMD ["./ant"]

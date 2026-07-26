# Multi-stage build for hound
# ---- stage 1: build ----
FROM golang:1.22-alpine AS builder

WORKDIR /src

# cache go.mod deps
COPY go.mod ./
COPY go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(cat VERSION 2>/dev/null || echo dev)" \
    -o /hound ./cmd/hound

# ---- stage 2: runtime ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /hound /usr/local/bin/hound

EXPOSE 53/udp
EXPOSE 53/tcp
EXPOSE 8080/tcp

VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/hound"]

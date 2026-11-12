# syntax=docker/dockerfile:1.7

# ---------- build stage ----------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Trust extra CA certs (TLS-inspecting proxy / antivirus) if the host provides
# any under docker/certs/*.crt. Harmless no-op when the directory is empty.
COPY docker/certs/ /usr/local/share/ca-certificates/
RUN apk add --no-cache ca-certificates && update-ca-certificates 2>/dev/null

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate

# ---------- runtime stage ----------
FROM alpine:3.21

# wget (busybox) is used by the container HEALTHCHECK; tzdata for Asia/Tokyo.
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 10001 -S app && adduser -u 10001 -S -G app app

WORKDIR /app
COPY --from=builder /out/api /out/migrate /app/

USER app:app
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1

ENTRYPOINT ["/app/api"]

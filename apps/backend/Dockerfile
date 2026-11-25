# ── Build stage ──────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binary: the runtime image has no libc to link against.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/backend .

# ── Runtime stage ────────────────────────────────────────────────────
FROM docker:cli AS dockercli

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata sqlite

# The docker CLI is needed to manage one cloudflared container per tunnel.
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker

WORKDIR /app
COPY --from=build /out/backend /app/backend

# /app/data is a named volume. It holds app.db (+ WAL), .jwt_secret and
# encryption.key. Losing it means losing every stored Cloudflare credential,
# so it must never be baked into the image or written to an anonymous volume.
VOLUME ["/app/data"]

ENV DATA_DIR=/app/data \
    DB_PATH=/app/data/app.db \
    PORT=3001 \
    CONNECTOR_MODE=docker

EXPOSE 3001
ENTRYPOINT ["/app/backend"]

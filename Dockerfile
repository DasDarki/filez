# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Pure-Go SQLite (modernc.org/sqlite) means no CGO — build a fully static,
# self-contained binary. The web UI and fonts are embedded via go:embed.
ARG VERSION=docker
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/filez-server ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget su-exec tzdata \
    && adduser -D -u 10001 filez

COPY --from=build /out/filez-server /usr/local/bin/filez-server
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Sensible container defaults; override any of these in Coolify's env settings.
ENV PORT=8080 \
    DATA_DIR=/data \
    PUBLIC=true \
    TRUST_PROXY=true

# Persist metadata (SQLite) and stored files. Mount a Coolify volume here.
VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${PORT}/api/info" >/dev/null 2>&1 || exit 1

# The entrypoint fixes /data ownership (works with root- or user-owned volumes)
# and then drops privileges to the unprivileged "filez" user.
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["filez-server"]

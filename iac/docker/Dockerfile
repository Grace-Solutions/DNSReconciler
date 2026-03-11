# ---- Build stage ----
FROM golang:1.22-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache dependency downloads
COPY src/go.mod ./
RUN go mod download

# Copy source and build a fully static binary
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /dnsreconciler ./cmd/dnsreconciler

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata su-exec \
    && addgroup -g 911 -S dnsreconciler \
    && adduser -u 911 -G dnsreconciler -S -H -D dnsreconciler

# Default PUID/PGID — override at runtime to match host ownership
ENV PUID=911
ENV PGID=911

# Prepare mount points per §8.2
RUN mkdir -p /config /state \
    && chown dnsreconciler:dnsreconciler /config /state

COPY --from=build /dnsreconciler /usr/local/bin/dnsreconciler
COPY iac/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Entrypoint runs as root to fix permissions, then drops to PUID:PGID
ENTRYPOINT ["/entrypoint.sh"]
CMD ["dnsreconciler", "-config", "/config/config.json"]


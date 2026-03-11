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

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S dnsreconciler \
    && adduser -S -G dnsreconciler dnsreconciler

# Prepare mount points per §8.2
RUN mkdir -p /config /state \
    && chown dnsreconciler:dnsreconciler /config /state

COPY --from=build /dnsreconciler /usr/local/bin/dnsreconciler

USER dnsreconciler

ENTRYPOINT ["dnsreconciler"]
CMD ["-config", "/config/config.json"]


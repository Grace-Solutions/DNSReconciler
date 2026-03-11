# Changelog

## Unreleased

### Added

- **Project foundation** — Go module, entrypoint, package layout aligned to `docs/DesignSpecification.md`.
- **Core models** — Centralized logging, config/state models, provider contracts, and service lifecycle abstractions.
- **Runtime context resolver** — Hostname, stable node ID (machine UUID → hostname fallback), OS/arch detection, public IP resolution via multiple upstream services.
- **Address selection engine** — Priority-based address resolution with CIDR allow/deny filtering and per-record overrides. Supports `publicIPv4`, `publicIPv6`, `rfc1918IPv4`, `cgnatIPv4`, `interfaceIPv4/v6`, and `explicitIPv4/v6` source types.
- **Variable expansion engine** — 14 built-in variables (`${HOSTNAME}`, `${NODE_ID}`, `${SELECTED_IPV4}`, `${PUBLIC_IPV4}`, `${OS}`, etc.) expanded in record names, content, comments, and tags.
- **Defaults merging** — Built-in → provider → global → per-record inheritance chain.
- **Reconciliation pipeline** — Full startup flow and per-record reconciliation with fingerprint-based drift detection and state persistence.

### Providers

- **Cloudflare** — API v4 with structured tags, comments, proxied flag support.
- **Technitium** — DNS Server API with comment-based ownership.
- **PowerDNS** — Authoritative Server API with RRset operations and comment-based ownership.
- **AWS Route53** — XML API with manual AWS Signature V4 authentication (zero external dependencies). State-file ownership.
- **Azure DNS** — REST API with OAuth2 client credentials flow. Record set metadata used as structured tags for ownership.

### Infrastructure

- **Scheduler** — Interval-based reconciliation loop with jitter, IP-change trigger, and signal-driven shutdown. Supports `--once` flag for single-pass mode.
- **Cleanup on shutdown** — Graceful shutdown deletes owned records when `cleanupOnShutdown` is enabled.
- **Service lifecycle managers** — Platform-specific service install/remove/start/stop for systemd (Linux), Windows Services (sc.exe), and launchd (macOS).
- **Cross-platform binaries** — `CGO_ENABLED=0` static builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 published in `Binaries/`.

### Docker

- **Dockerfile** — Multi-stage Go build producing a minimal container.
- **docker-compose.yml** — Environment variable injection for credentials with commented-out Docker secrets pattern. Directory bind mounts for `/config` and `/state`. `restart: unless-stopped` as the service manager.
- **PUID/PGID support** — Entrypoint script with `su-exec` for user/group mapping.
- **Per-provider example configs** — Standalone config files for each provider (`config.cloudflare.json`, `config.technitium.json`, `config.powerdns.json`, `config.route53.json`, `config.azure.json`).

### Configuration

- **Auto-generated default config** — When no config file exists, a valid default is written to disk as a starting template.
- **Interval override** — Reconcile interval configurable via `-interval` CLI flag or `RECONCILE_INTERVAL_SECONDS` environment variable (CLI > env > config file).
- **Credential resolution** — `env:VAR_NAME`, `file:/path`, or direct values supported in all provider credential fields.
- **Disabled AAAA records** — Each example config includes a disabled IPv6 record template ready to enable.
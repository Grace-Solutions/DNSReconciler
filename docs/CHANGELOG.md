# Changelog

## 2026.03.18.1515

### Added

- **Container service discovery** — automatic DNS registration for containers on L2-routable networks (IPVLAN and MACVLAN). Queries Docker and Podman Unix sockets directly via the Engine REST API — no CLI dependency required.
- **`containerRecords[]` config section** — template-based record definitions that expand once per discovered container. Supports all standard variables plus container-specific ones.
- **Container variables** — `${CONTAINER_NAME}`, `${CONTAINER_ID}`, `${CONTAINER_IP}`, `${CONTAINER_IMAGE}`, and label lookups via `${LABEL:key}` syntax.
- **Auto-injected ownership tags** — every container-generated record receives `nodeId`, `containerName`, and `containerId` tags automatically. Providers without structured tag support (Technitium, PowerDNS) have these stripped before the API call.
- **Deterministic record IDs** — container records use `SHA-256(templateId + containerId)` for stable ownership tracking across restarts.
- **`labelFilter` support** — container record templates can filter containers by label key/value pairs.
- **Docker socket mount** — `docker-compose.yml` updated with optional read-only Docker socket mount for container discovery.

### Changed

- **Address selection** — virtual bridge interfaces (`docker0`, `br-*`, `veth*`, `virbr*`, `cni*`, `flannel*`, `calico*`, `weave*`) are now excluded from RFC 1918 and CGNAT address detection to prevent container bridge IPs from being selected as the host address.
- **Example configs** — all provider example configs now include `containerRecords` examples alternating between IPVLAN and MACVLAN use cases.
- **README** — added Container Service Discovery section, `containerRecords[]` schema, container variable reference, and updated project structure.

### Infrastructure

- **`containerrt` package** — new package for container runtime discovery with `DockerClient` and `PodmanClient` implementations using Unix socket HTTP transport.
- **`reconcile/container.go`** — template expansion logic that generates concrete `RecordTemplate` instances from `ContainerRecordTemplate` × discovered containers.
- **`.gitignore`** — added `state.json` exclusion.

---

## Unreleased (initial)

### Added

- **Project foundation** — Go module, entrypoint, package layout aligned to `docs/DesignSpecification.md`.
- **Core models** — Centralized logging, config/state models, provider contracts, and service lifecycle abstractions.
- **Runtime context resolver** — Hostname, stable node ID (machine UUID → hostname fallback), OS/arch detection, public IP resolution via multiple upstream services.
- **Address selection engine** — Priority-based address resolution with CIDR allow/deny filtering and per-record overrides. Supports `publicIPv4`, `publicIPv6`, `rfc1918IPv4`, `cgnatIPv4`, `interfaceIPv4/v6`, and `explicitIPv4/v6` source types.
- **Variable expansion engine** — 14 built-in variables (`${HOSTNAME}`, `${NODE_ID}`, `${SELECTED_IPV4}`, `${PUBLIC_IPV4}`, `${OS}`, etc.) expanded in record names, content, comments, and tags.
- **Defaults merging** — Built-in → provider → per-record inheritance chain (three levels).
- **Reconciliation pipeline** — Full startup flow and per-record reconciliation with fingerprint-based drift detection and state persistence.

### Providers

- **Cloudflare** — API v4 with structured tags, comments, proxied flag support. Automatic plan detection (free plan falls back to comment-based ownership). Plan status cached with 24h TTL via `CapabilityRefresher` interface.
- **Technitium** — DNS Server API with comment-based ownership.
- **PowerDNS** — Authoritative Server API with RRset operations and comment-based ownership.
- **AWS Route53** — XML API with manual AWS Signature V4 authentication (zero external dependencies). State-file ownership.
- **Azure DNS** — REST API with OAuth2 client credentials flow. Record set metadata used as structured tags for ownership.

### Infrastructure

- **Scheduler** — Interval-based reconciliation loop with jitter, IP-change trigger, and signal-driven shutdown. Supports `--once` flag for single-pass mode.
- **Cleanup on shutdown** — Graceful shutdown deletes owned records when `cleanupOnShutdown` is enabled.
- **Service lifecycle managers** — Platform-specific service `install`, `uninstall`, `start`, `stop`, and `init` (install + start) for systemd (Linux), Windows Services (sc.exe), and launchd (macOS). All operations are idempotent.
- **Cross-platform binaries** — `CGO_ENABLED=0` static builds for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 published in `Binaries/`.
- **Config hot-reload** — Polling-based file watcher (5s interval) detects `config.json` modifications. Configuration and providers are atomically swapped via `runState` struct with `sync.RWMutex`. Invalid configs are logged and skipped — the previous valid config continues running.
- **Centralized HTTP logging** — `APIClient` logs every HTTP request/response at `Debug` level with method, path, status code, and duration. Azure and Route53 custom HTTP methods log identically. Non-2xx responses log at `Warning` level.
- **Provider operation logging** — All provider CRUD operations (`ListRecords`, `CreateRecord`, `UpdateRecord`, `DeleteRecord`) log at `Information` level with record type, name, and content details. `ListRecords` logs at `Debug` level with result counts and filter parameters.

### Docker

- **Dockerfile** — Multi-stage Go build producing a minimal container.
- **docker-compose.yml** — Environment variable injection for credentials with commented-out Docker secrets pattern. Directory bind mounts for `/config` and `/state`. `restart: unless-stopped` as the service manager.
- **PUID/PGID support** — Entrypoint script with `su-exec` for user/group mapping.
- **Per-provider example configs** — Standalone config files for each provider (`config.cloudflare.json`, `config.technitium.json`, `config.powerdns.json`, `config.route53.json`, `config.azure.json`) in the `example/` directory.

### Configuration

- **Instance-based provider model** — Providers are configured as an array with UUIDs and friendly names. Records reference providers by `providerId` (ID or friendly name). Multiple instances of the same provider type are supported.
- **Flattened provider defaults** — TTL, proxied, comment, tags, and zone live directly on the provider entry — no nested `defaults` wrapper.
- **Record identifiers** — Records use `recordId` (UUID) instead of `id`. Field order: `providerId`, `recordId`, `enabled`, `type`, `name`, `content`.
- **Implicit ownership** — Default ownership model is `perNode`. The `ownership` field is removed from provider level; per-record override is still available.
- **Zone inheritance** — Records inherit `zone` from their provider when not set explicitly.
- **Auto-generated default config** — When no config file exists, a valid default with Cloudflare provider and sample records is written to disk.
- **Interval override** — Reconcile interval configurable via `-interval` CLI flag or `RECONCILE_INTERVAL_SECONDS` environment variable (CLI > env > config file).
- **Credential resolution** — `env:VAR_NAME`, `file:/path`, or direct values supported in all provider credential fields.
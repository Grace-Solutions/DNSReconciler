# Dynamic DNS Reconciler Specification

## 1. Purpose

Build a cross-platform dynamic DNS reconciliation service that allows nodes to automatically create, update, and remove their own DNS records based on shared configuration and local runtime state.

The service is intended for environments such as:

* Docker Swarm global services
* VPS fleets
* bare metal nodes
* VMs
* overlay/mesh networks
* hybrid public/private DNS topologies

The first practical use case is automatic round-robin style DNS registration for distributed services, where each node publishes its own reachable address without requiring manual DNS record management.
, uso 
The design must be extensible from day one so that it can support multiple DNS providers and multiple address source strategies.

## 2. Primary goals

The service must:

* reconcile desired DNS state against actual provider state
* allow each node to manage only the records it owns
* support a shared JSON config consumed by many nodes
* support one or more record definitions
* support global defaults and per-record overrides
* support variable expansion
* support multiple DNS providers
* support public, RFC1918, and CGNAT/overlay addresses
* support local state for idempotency and cleanup
* run as a single binary
* run in Docker using the same binary
* be safe, deterministic, and debuggable

## 3. Non-goals

This service is not:

* a full global traffic manager
* a CDN
* a service mesh
* an orchestrator
* a full health-checked load balancer
* a replacement for internal service discovery systems

It is a DNS reconciliation engine.

## 4. Core design principles

### 4.1 Provider-agnostic core

The core reconciliation engine must not contain Cloudflare-specific, Technitium-specific, or PowerDNS-specific assumptions.

### 4.2 Single codebase, single runtime story

The same executable must run:

* directly on the host
* inside Docker
* as a service where applicable

No separate implementation for containers.

### 4.3 Safe ownership

A node must only create, update, or delete records it can prove it owns.

### 4.4 Idempotent reconciliation

Repeated runs must converge on the same correct state.

### 4.5 Extensibility first

The internal models and provider APIs must be designed so new DNS providers and new address sources can be added without reworking the core engine.

## 5. Supported providers

### 5.1 Required providers

Initial supported provider set:

* Cloudflare
* Technitium DNS Server
* PowerDNS Authoritative Server

### 5.2 Future provider targets

The architecture should make it straightforward to add:

* Route53
* Azure DNS
* Google Cloud DNS
* RFC2136-compatible DNS
* custom/internal DNS APIs

## 6. Supported address classes

The service must support selecting DNS record content from multiple address classes.

### 6.1 Required address classes

* public IPv4
* public IPv6
* RFC1918 IPv4
* CGNAT IPv4
* interface-specific IPv4
* interface-specific IPv6
* explicit configured address

### 6.2 Intended use cases

This allows records to point at:

* public internet ingress IPs
* internal LAN addresses
* mesh/VPN addresses such as Tailscale-style or NetBird-style interfaces
* private inter-node service addresses

The system must not assume public IP is always the correct answer.

## 7. High-level architecture

The system is composed of the following components.

### 7.1 Core Engine

Responsible for:

* loading config
* validating config
* resolving runtime context
* merging defaults
* expanding variables
* selecting addresses
* reconciling desired state vs provider state
* persisting local state
* running scheduled reconciliation

### 7.2 Provider Interface

Abstract contract implemented by each DNS backend.

### 7.3 Address Resolver

Resolves addresses from configured address sources using global or per-record priority rules.

### 7.4 Runtime Context Resolver

Collects runtime values such as:

* hostname
* node ID
* OS
* architecture
* public IPs
* interface IPs
* environment variables
* machine or instance metadata where available

### 7.5 State Store

Persists local state to disk.

### 7.6 Variable Expansion Engine

Expands variables in names, content, comments, tags, and selected provider options.

### 7.7 Scheduler

Triggers reconciliation at startup and on interval, with optional IP-change-based triggers.

### 7.8 Centralized Logger

Single logging function or logger service used globally across all components.

## 8. Packaging and runtime requirements

## 8.1 Binary requirements

The v1 implementation language is Go.

The service must build as a single executable.

Target platforms should include:

* Linux amd64
* Linux arm64
* Windows amd64
* Windows arm64 where practical
* macOS amd64
* macOS arm64

The build and release process must produce Windows, macOS, and Linux binaries from the same codebase.

The binary should be static where practical.

For platforms where fully static linking is practical, releases should prefer static builds.

For platforms where fully static linking is not practical, the deliverable must still be a single self-contained executable with no separate runtime-specific implementation.

## 8.2 Docker requirements

The service must run in Docker using the same binary as bare metal.

Typical mount structure:

* `/config/config.json`
* `/state/state.json`

Credentials must be injectable through:

* environment variables
* mounted secrets
* file paths

## 8.3 Service support

Native service support is required for supported host platforms.

Initial host service targets should include:

* systemd on Linux
* Windows service on Windows
* launchd on macOS

## 8.4 Service lifecycle requirements

The single executable must support service lifecycle operations for host environments.

Required lifecycle actions:

* install
* remove
* start
* stop

Lifecycle operations must be idempotent.

If the requested service state is already satisfied, the command should complete safely as a no-op or deterministic update path with clear logging.

The service runtime must use the same executable and core configuration semantics as foreground and Docker execution.

## 9. Provider abstraction

All provider-specific logic must be isolated behind a common provider interface.

### 9.1 Required conceptual provider operations

* validate provider configuration
* list records or candidate records
* create record
* update record
* delete record
* normalize provider records into internal models
* report provider capabilities

### 9.2 Provider capabilities

Each provider must expose capabilities such as:

* supports comments
* supports structured tags
* supports server-side filtering by comment
* supports server-side filtering by tag
* supports per-record updates
* supports RRset-oriented updates
* supports wildcard records
* supports proxied or equivalent flags
* supports batch changes

The core engine must use these capabilities to adapt behavior per provider.

## 10. Internal record model

The internal desired-state model must be provider-neutral.

Minimum fields:

* provider
* zone
* type
* name
* content
* ttl
* enabled
* proxied or equivalent if supported
* comment
* tags
* ownership mode
* record template ID
* provider record ID
* desired state fingerprint

## 11. Ownership model

Ownership is the most important safety mechanism.

### 11.1 Canonical ownership

Ownership must be machine-readable and deterministic.

Preferred ownership metadata should include:

* managed-by
* node-id
* node-hostname
* record-template-id
* service name where applicable

### 11.2 Comments vs tags

If a provider supports tags, tags should be used as the canonical ownership mechanism.

Comments are informational and human-friendly, not authoritative.

### 11.3 Provider-specific ownership handling

Because providers differ, ownership implementation must adapt:

* Cloudflare: tags preferred, comments optional
* Technitium: use provider-supported metadata or deterministic matching where metadata is limited
* PowerDNS: handle ownership carefully at RRset level where needed

### 11.4 Stable node identity

Node identity should be derived in this order:

1. explicitly configured node ID
2. machine UUID/system UUID
3. stable instance/container identity if reliable
4. hostname as last resort

Hostnames are less reliable than UUID-like identifiers.

## 12. Address source priority model

The system must support global address-selection priority.

### 12.1 Priority behavior

Global address selection must evaluate sources in ascending priority order:

* 1 = first choice
* 2 = second choice
* 3 = third choice
* 4 = fourth choice

The first usable address wins.

If no address source yields a usable result, the system must log the condition and skip processing that record.

### 12.2 Global default address source model

The config must allow a global address source list.

Example conceptual source types:

* publicIPv4
* publicIPv6
* interfaceIPv4
* interfaceIPv6
* rfc1918IPv4
* cgnatIPv4
* explicitIPv4
* explicitIPv6

### 12.3 Source constraints

Each address source may optionally support:

* interface name
* allowed CIDRs
* denied CIDRs
* address family requirements

## 13. Record-level address override

Global priority must be the default behavior, but records must be able to override address selection.

This is required because one config may need:

* public records for one hostname
* internal RFC1918 records for another
* overlay/mesh records for another

## 14. Configuration format

## 14.1 Format

JSON for v1.

## 14.2 Top-level structure

```json
{
  "version": 1,
  "providerDefaults": {},
  "runtime": {},
  "network": {},
  "defaults": {},
  "records": []
}
```

## 15. Top-level config sections

### 15.1 `version`

Integer schema version.

### 15.2 `providerDefaults`

Provider-specific default settings such as API endpoints, zone defaults, server IDs, or provider behavior options.

### 15.3 `runtime`

Runtime behavior such as:

* reconcile interval
* state path
* cleanup on shutdown
* log level
* dry run
* retry settings

### 15.4 `network`

Global address source configuration and selection priorities.

### 15.5 `defaults`

Default values inherited by records.

### 15.6 `records`

Array of desired record templates.

## 16. Record template schema

Each record template should support the following fields.

* `id`
* `enabled`
* `provider`
* `ownership`
* `zone`
* `type`
* `name`
* `content`
* `ttl`
* `proxied`
* `comment`
* `tags`
* `matchLabels`
* `addressSelection`
* `ipFamily`

## 17. Record field definitions

### `id`

Unique identifier within the config.

### `enabled`

Boolean controlling whether the record should be processed.

### `provider`

DNS provider name.

### `ownership`

One of:

* `perNode`
* `singleton`
* `manual`
* `disabled`

Default for this system is `perNode`.

### `zone`

Target DNS zone.

### `type`

Initially required support:

* `A`
* `AAAA`

Later possible support:

* `TXT`
* `CNAME`
* `SRV`

### `name`

Specific hostname or wildcard hostname.

### `content`

Record value after variable expansion.

### `ttl`

TTL value subject to provider constraints.

### `proxied`

Provider-specific proxy flag where supported.

### `comment`

Human-readable metadata string.

### `tags`

Array of objects with:

* `name`
* `value`

### `matchLabels`

Optional node matching filter.

### `addressSelection`

Optional per-record override of global address source selection.

### `ipFamily`

Optional family preference such as IPv4, IPv6, or dual.

## 18. Defaults and inheritance

Values should be resolved in this order:

1. built-in defaults
2. provider defaults
3. global defaults
4. per-record values

This reduces duplication and keeps shared config maintainable.

## 19. Variable expansion

The system must support variable expansion in:

* record names
* content
* comments
* tag values
* selected provider settings where applicable

### 19.1 Required variables

* `${HOSTNAME}`
* `${NODE_ID}`
* `${OS}`
* `${ARCH}`
* `${PUBLIC_IPV4}`
* `${PUBLIC_IPV6}`
* `${RFC1918_IPV4}`
* `${CGNAT_IPV4}`
* `${SELECTED_IPV4}`
* `${SELECTED_IPV6}`
* `${SERVICE_NAME}`
* `${STACK_NAME}`
* `${ZONE}`
* `${RECORD_ID}`

### 19.2 Variable behavior

If a required variable cannot be resolved:

* fail the specific record
* log clearly
* continue processing other records

## 20. Local state

A local state file is required.

### 20.1 Purpose

The state file provides runtime memory for:

* last seen addresses
* known provider record IDs
* config fingerprint
* last reconciliation success
* cleanup hints
* previously owned records

### 20.2 Format

JSON for v1.

### 20.3 Suggested fields

* node ID
* hostname
* config checksum
* public IPv4/IPv6 last seen
* selected address snapshots
* provider record IDs by template
* desired state fingerprint by template
* last success timestamp
* pending cleanup items

## 21. Reconciliation algorithm

## 21.1 Startup flow

1. load config
2. validate config
3. initialize centralized logger
4. load local state
5. resolve runtime context
6. resolve address sources
7. merge defaults
8. expand templates
9. reconcile records
10. persist state
11. enter scheduled loop

## 21.2 Per-record reconciliation

For each enabled applicable record:

1. determine applicable provider
2. determine address source policy
3. resolve selected address by ascending priority
4. if no address is found, log warning and skip
5. construct desired record
6. determine ownership filters
7. query provider for candidate records
8. identify owned record(s)
9. compare desired vs actual
10. perform action:

* no-op
* create
* update
* delete

11. update local state

## 21.3 Allowed actions

A record reconciliation cycle may result in:

* no change
* create new owned record
* update existing owned record
* delete owned record if disabled or no longer applicable
* skip with warning if address or prerequisites unavailable

## 22. Address resolution behavior

## 22.1 Global selection

If the record does not override address selection:

* use global `network.addressSources`
* evaluate priorities 1 to 4
* choose first valid result

## 22.2 Record override

If the record defines `addressSelection`:

* use that source list instead
* evaluate priorities 1 to 4
* choose first valid result

## 22.3 Failure behavior

If none of the configured address sources returns a usable address:

* log a warning
* skip the record
* continue processing the rest

## 23. Logging specification

Logging must be centralized and globally reused.

### 23.1 Required rule

All code paths must log through one centralized logging function or logger service.

No direct console printing outside the logger.

### 23.2 Required format

Every log line must use this exact structure:

```text
[UTC timestamp] - [Level] - Message
```

### 23.3 Examples

```text
[2026-03-10 18:42:11.223Z] - [Information] - Reconciled record api.example.com with address 203.0.113.10
[2026-03-10 18:42:12.004Z] - [Warning] - No usable address source found for record mesh-api
[2026-03-10 18:42:12.220Z] - [Error] - Provider update failed for record public-api: unauthorized
```

### 23.4 Supported levels

* Trace
* Debug
* Information
* Warning
* Error
* Critical

### 23.5 Logging rules

* timestamps must always be UTC
* formatting must be globally consistent
* provider adapters must use the same logger
* secrets must never be logged

## 24. Error handling and retries

The service must fail per record where possible, not globally.

Examples:

* bad variable in one record should not stop other records
* provider error for one record should be logged and retried later
* malformed record config should identify the record ID clearly

Retry behavior should include:

* bounded retries
* backoff
* jitter
* provider-specific rate-limit awareness where relevant

## 25. Scheduling behavior

The reconciler should run:

* at startup
* on a fixed interval
* optionally on detected IP change
* optionally on manual trigger or signal

A configurable interval with jitter is required to avoid startup stampedes when many nodes start simultaneously.

## 26. Cleanup behavior

### 26.1 Graceful shutdown

If enabled, the agent should delete records it owns on clean shutdown.

### 26.2 Unclean shutdown

On next startup, the agent should use local state plus provider ownership checks to determine whether stale records should be refreshed, kept, or removed.

### 26.3 Future stale handling

Future versions may support:

* stale age thresholds
* lease-like ownership
* heartbeat metadata

## 27. Security requirements

### 27.1 Least privilege

Provider credentials must be scoped as narrowly as practical.

### 27.2 Secret handling

Credentials must be accepted via:

* environment variables
* secret files
* mounted file paths

Secrets must never appear in logs.

### 27.3 Ownership safety

The service must never delete or modify records it cannot prove it owns.

## 28. Observability

### 28.1 Logging

Required.

### 28.2 Metrics

Optional but recommended in later phases.

Potential metrics include:

* reconciliation runs
* create/update/delete counts
* provider failures
* address resolution failures
* state load/save failures

### 28.3 Dry-run mode

A dry-run mode is required so operators can preview actions without mutating providers.

### 28.4 Build validation and commit discipline

Implementation work must be validated before it is committed.

Required workflow rules:

* each commit must follow a successful relevant build
* code changes should also pass the smallest relevant automated test set where available
* commits should remain small and logically scoped rather than accumulate unrelated work

## 29. Example consolidated config

```json
{
  "version": 1,
  "providerDefaults": {
    "cloudflare": {
      "zone": "example.com"
    },
    "powerdns": {
      "serverId": "localhost"
    },
    "technitium": {}
  },
  "runtime": {
    "schedule": "0 0 */4 * * *",
    "jitter": "auto",
    "timezone": "UTC",
    "statePath": "./state.json",
    "cleanupOnShutdown": true,
    "logLevel": "Information",
    "dryRun": false
  },
  "network": {
    "addressSources": [
      {
        "priority": 1,
        "type": "publicIPv4",
        "enabled": true
      },
      {
        "priority": 2,
        "type": "interfaceIPv4",
        "enabled": true,
        "interfaceName": "tailscale0",
        "allowRanges": ["100.64.0.0/10"]
      },
      {
        "priority": 3,
        "type": "rfc1918IPv4",
        "enabled": true
      },
      {
        "priority": 4,
        "type": "cgnatIPv4",
        "enabled": true
      }
    ]
  },
  "defaults": {
    "enabled": true,
    "ownership": "perNode",
    "ttl": 120,
    "proxied": false,
    "comment": "Managed by dnsreconciler on ${HOSTNAME}",
    "tags": [
      { "name": "managed-by", "value": "dnsreconciler" },
      { "name": "node-id", "value": "${NODE_ID}" },
      { "name": "node-hostname", "value": "${HOSTNAME}" }
    ]
  },
  "records": [
    {
      "id": "public-api",
      "enabled": true,
      "provider": "cloudflare",
      "zone": "example.com",
      "type": "A",
      "name": "api.example.com",
      "content": "${SELECTED_IPV4}",
      "tags": [
        { "name": "service", "value": "api" }
      ]
    },
    {
      "id": "mesh-api",
      "enabled": true,
      "provider": "powerdns",
      "zone": "mesh.example.internal",
      "type": "A",
      "name": "api.mesh.example.internal",
      "content": "${SELECTED_IPV4}",
      "addressSelection": {
        "useGlobalDefaults": false,
        "sources": [
          {
            "priority": 1,
            "type": "interfaceIPv4",
            "enabled": true,
            "interfaceName": "tailscale0",
            "allowRanges": ["100.64.0.0/10"]
          },
          {
            "priority": 2,
            "type": "rfc1918IPv4",
            "enabled": true
          }
        ]
      }
    },
    {
      "id": "internal-node",
      "enabled": true,
      "provider": "technitium",
      "zone": "internal.example.local",
      "type": "A",
      "name": "${HOSTNAME}.internal.example.local",
      "content": "${SELECTED_IPV4}"
    }
  ]
}
```

## 30. Future roadmap

### 30.1 Near-term

* TXT and CNAME support
* config hot reload
* better stale record cleanup
* provider batching
* more interface/address source types

### 30.2 Mid-term

* singleton record coordination
* leader election
* embedded status endpoint
* metrics endpoint
* remote config sources

### 30.3 Long-term

* plugin model for providers
* plugin model for address resolvers
* health-aware record publishing
* signed state and config validation

## 31. Final hard requirements

These are mandatory design rules:

1. support Cloudflare, Technitium, and PowerDNS through a common provider abstraction
2. support public, RFC1918, and CGNAT/overlay address selection
3. support global priority-based address selection from 1 through 4
4. if no address is found, log and skip the record
5. use centralized logging everywhere
6. all logs must use the exact format `[UTC timestamp] - [Level] - Message`
7. maintain local state for idempotency and cleanup
8. run as a single executable
9. run in Docker using the same executable
10. implement the service in Go
11. produce Windows, macOS, and Linux builds from the same codebase
12. support host service lifecycle operations for install, remove, start, and stop
13. make service lifecycle operations idempotent
14. keep the design extensible for more providers and more address sources
15. only create commits after successful relevant build validation

If you want, I can turn this next into a developer implementation spec with project structure, config JSON schema, CLI arguments, and provider interface definitions in Go.

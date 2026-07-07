# Architecture

## Response Pipeline

Every HTTP response passes through a fixed, ordered sequence of filters:

```
Status
  ↓
Headers
  ↓
Content-Length
  ↓
Body
```

**Rule:** cheapest checks execute first.

A response that fails any stage is immediately discarded (`resp.Body.Close()`). Later stages never execute. This minimises CPU, memory, and I/O usage per rejected response.

| Stage | Cost | Current behaviour |
|---|---|---|
| Status | Integer comparison | Exclude list applied |
| Headers | Header map lookup | Placeholder (always accepts) |
| Content-Length | Single header read | Placeholder (always accepts) |
| Body | Bounded I/O read | At most 4096 bytes, then discarded |

Future filters slot into the appropriate stage without reordering the pipeline.

---

## Connection Reuse Policy

Searchit uses a single shared `*http.Client` and `*http.Transport` across all workers in a pool. Key settings:

| Setting | Value | Reason |
|---|---|---|
| `MaxIdleConns` | 1000 | Avoid connection starvation across many hosts |
| `MaxIdleConnsPerHost` | 100 | Saturate a single target without per-request TCP handshakes |
| `IdleConnTimeout` | 90 s | Evict stale connections before the server RSTs them |
| `KeepAlive` | enabled (default) | Reuse TCP connections across requests |

---

## Early Rejection vs. Keep-Alive Utilisation

Searchit reads **at most 4096 bytes** of every accepted response body, then closes it.

When a response body is not fully consumed, Go's HTTP client cannot return the underlying TCP connection to the keep-alive pool. The connection is instead closed.

This is an **intentional tradeoff**:

```
Early rejection
    >
Maximum connection reuse
```

The rationale:

- Content discovery scans issue thousands of requests per second. Most responses are rejected at the status or header stage and never reach the body read, so keep-alive works well for the majority of traffic.
- For the small fraction of responses that pass all filters, discarding after 4096 bytes is the correct behaviour: storing large bodies would degrade throughput and increase memory pressure.
- Forcing a full body drain on every accepted response to preserve keep-alive would impose unpredictable latency and memory usage depending on response size.

Searchit optimises for:

- **Throughput** — maximise requests per second
- **Memory usage** — never buffer full response bodies
- **Predictable behaviour** — bounded read regardless of server response size

rather than perfect keep-alive utilisation on every accepted response.

---

## Orchestration Model

```
Producer
  ↓ (jobs channel)
Worker Pool  ← N goroutines
  ↓ (results channel)
Consumer
```

- **Producer** — generates `Job` values and closes the jobs channel when done. Respects context cancellation.
- **Worker** — pulls jobs, executes the pipeline, emits `Result` values.
- **Scanner** — public facade that wires a `Producer` and a pool and returns the results channel.
- **Pool** — manages the worker goroutine lifecycle; closes results when all workers exit.

Context cancellation propagates from `Scanner.Scan` → `Start` → `Worker` → `http.NewRequestWithContext`, stopping in-flight requests without goroutine leaks.

---

## Profile Overlay Architecture

Profiles in Searchit are generic, tool-agnostic resources designed to be shared across future tools (scan, fuzz, subdomain, workflow, report, etc.).

### Generic Core

The `internal/profile` package acts as a generic loader and registry. It is responsible for:
- Profile metadata unmarshaling (name, tool, description)
- Storage resolution and lookup (embedded profiles and user overrides)
- Namespace structure verification (e.g. `scan/quick`)

To prevent circular dependencies and maintain decoupling, the profile package does not import or know about any tool-specific configuration packages (like `internal/config`). Instead, it loads the profile's tool configuration section as a raw `yaml.Node` and exposes a `Decode(v any)` method.

### Tool-Specific Overlays

Each tool defines its own overlay configuration schema where all fields are pointers (e.g. `internal/profile/scan/overlay.go`). Using pointers allows fields to be optional (`nil` represents a field that is not present in the profile overlay).

The tool-specific package decodes the generic profile config section and merges it onto the tool's runtime configuration using an explicit `Apply` function (e.g. `internal/profile/scan/apply.go`):

```
+-------------------+
|  profile package  | loads generic metadata & raw yaml.Node config
+---------+---------+
          | Decode(any)
          v
+-------------------+
|   scan package    | interprets raw yaml.Node config into scan.Overlay
+---------+---------+
          | Apply(config, overlay)
          v
+-------------------+
|   config package  | updates config.Config with non-nil overlay fields
+-------------------+
```

### Precedence Order

When starting a scan, the configuration layer is resolved progressively (left-to-right), where the last layer always wins:

1. **Default Configuration** (built-in defaults)
2. **Profiles** (applied left-to-right if multiple profiles are defined via `--profile`)
3. **Config file** (future)
4. **CLI Flags** (explicitly specified by the user, highest priority)


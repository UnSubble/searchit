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

---

## Creating Profiles

The `profile create` subcommand establishes a new user profile skeleton:

```bash
searchit profile create scan/myprofile
```

### Storage Locations

User-created profiles are stored on the filesystem under the user's home configuration directory:
- **Linux/macOS**: `~/.config/searchit/profiles/`
- **Windows**: `%AppData%/searchit/profiles/`

The namespace (first segment of the name, e.g. `scan/` or `fuzz/`) is mandatory and determines the tool this profile applies to. A subdirectory is automatically created under the profiles directory to store the new profile (e.g. `~/.config/searchit/profiles/scan/myprofile.yaml`).

### Next Steps

Profile editing, merging, and advanced fields will be introduced in subsequent milestones.

---

## Profile Validation

Searchit profiles are validated using a modular, two-stage validation model:

1. **Generic Validation**: Checks the global shape and metadata of the profile document, verifying version schema, name consistency, tool configuration namespaces, and syntax.
2. **Tool-specific Validation**: Verifies configuration values unique to each tool (e.g. `scan` limits like threads or strategy choice) by decoding configuration elements via the `profile.Decode()` interface.

### Modular Extensibility

Future tools (like `fuzz`, `subdomain`, or `workflow`) register their own validators inside the registry of the generic `profile` package using:

```go
profile.Register(validatorInstance)
```

This keeps the profile registry completely decoupled and extensible.

---

## Explicit Validator Registration

Searchit registers validators explicitly during the application bootstrap phase, removing all implicit `init()`-based registration side effects.

### Advantages

- **Deterministic Startup**: Validator registration is fully controlled and occurs at a predictable point in the application lifecycle.
- **Simpler Testing**: Tests can easily register mock validators, register builtin validators, or assert duplicate registration errors programmatically.
- **Plugin-Friendly**: External packages and future plugins can register custom validators dynamically at runtime without relying on import side-effects.
- **Explicit Dependency Graph**: Eliminates circular imports and makes package dependencies clear.

### Registration Flow

1. During application initialization, the CLI package invokes `profile.RegisterBuiltinValidators()`.
2. The bootstrap package maps each validator by calling its `Tool() string` method:
   ```go
   // internal/profile/bootstrap.go
   func RegisterBuiltinValidators() error {
       if err := Register(scan.NewValidator()); err != nil {
           return err
       }
       return nil
   }
   ```
3. The registry validates that no duplicate validator is registered for the same tool namespace, throwing an error if a conflict occurs.

---

## Profile Schema Versioning

### Searchit Version vs. Profile Schema Version

Searchit distinguishes clearly between **Searchit Version** (the binary/release version of the CLI application, e.g., `v0.3.0-beta`) and **Profile Schema Version** (the version indicating the YAML format of the profile document itself, e.g., `schema: 1`).

Profile schemas evolve independently of Searchit releases. This separation ensures that a future Searchit release (e.g. `v2.x`) remains backward-compatible and capable of reading older profile schemas (e.g. `schema: 1`) without breaking.

### Decoder Registry

To support multiple schema versions dynamically and maintain a decoupled design, Searchit uses a **Decoder Registry**. The core `profile` package is agnostic to specific schema rules. It delegates decoding to registered `Decoder` implementations matching the schema version found in the YAML document header.

```go
type Decoder interface {
    Schema() int
    Decode([]byte) (*Profile, error)
}
```

### Decoding Flow

When a profile is loaded from disk or embedded storage, it goes through the following stages:

```
    YAML Document Data
           ↓
   [Header Detection] (Decodes only the "schema" field)
           ↓
   [Decoder Registry] (Looks up registered schema decoder)
           ↓
    [Schema Decoder]  (Translates YAML to Runtime Profile representation)
           ↓
    Runtime Profile   (Single, version-agnostic struct)
           ↓
      [Validation]    (Generic validation followed by Tool validation)
```

### Future Migrations

While not implemented in this milestone, future releases of Searchit may introduce a migration command:

```bash
searchit profile migrate <name> --to <schema_version>
```

This utility will allow users to upgrade custom profiles between schema versions programmatically.

---

## Editing Profiles

Searchit implements safe profile editing using the `searchit profile edit` command.

### Immutability of Embedded Profiles

Embedded/built-in profiles are completely read-only and embedded directly inside the Searchit binary. If a user attempts to edit a built-in profile (e.g. `scan/base` or `scan/quick`), Searchit will:
1. Load the embedded profile's configuration.
2. Initialize a local, writable copy inside the user's config directory (e.g., `~/.config/searchit/profiles/scan/base.yaml`).
3. Open the copy for editing.

Subsequent loads of that profile name will resolve to the user's custom override rather than the original embedded default.

### Safe Editing Workflow

To guarantee that broken profiles are never saved or overwrite valid configurations, editing uses an out-of-band validation loop:

```
                  Load Original Profile
                            ↓
               Write Temporary YAML File (*.yaml)
                            ↓
                 Launch Configured Editor
                            ↓
               [ User Edits and Saves File ]
                            ↓
                    Read Temporary File
                            ↓
               Run Complete Validation Loop
               (Generic validation & Tool validation)
                            ↓
           Is Valid? ───────────────────────── Is Invalid?
               ↓                                   ↓
      Replace Destination                    Keep Temporary File
    Atomically via os.Rename()            Print Validation Errors
               ↓                                   ↓
        [Session Done]                     User Can Resume Editing
```

### Editor Discovery

When launching the editor, Searchit checks the following sources in order:

1. **`VISUAL`** environment variable (e.g., `VISUAL="code --wait"`).
2. **`EDITOR`** environment variable (e.g., `EDITOR="vim"`).
3. **Platform Default**:
   - **Linux/macOS**: `nano` (looks up in system PATH).
   - **Windows**: `notepad.exe` (looks up in system PATH).

If no editor is configured and default tools are missing from the system path, editing will fail with a descriptive error.

---

## Statistics Engine

Searchit includes a decoupled runtime statistics collection system designed to gather performance and operational metrics during scans.

### UI-Agnostic Design

The statistics engine resides under the `internal/stats` package. It is strictly separated from presentation logic:
- It has **zero dependencies** on terminal rendering, Cobra, or command execution contexts.
- Its sole responsibility is the collection, aggregation, and snapshotting of raw runtime performance metrics.
- Future UI components, progress bars, or dashboards will consume these static snapshots to render updates to the terminal or other outputs.

### Concurrency and Performance

To avoid degrading scan performance, the statistics package is optimized for lock-free updates:
- **No Mutexes for Counters**: Simple metrics like requests sent, filtered, or bytes received are updated concurrently using lock-free `sync/atomic` operations.
- **Lock-free Status Counters**: Instead of using maps with mutexes or read-write locks, response status codes (e.g. `200`, `301`, `403`) are tracked using a pre-allocated fixed-size array of 1000 counters (covering codes `0-999`). Updates are performed using atomic additions to the respective array index, eliminating all lock contention and memory allocations during scans.
- **Immutable Snapshots**: The `Collector` exposes a `Snapshot()` function that generates an immutable `Snapshot` struct. Callers can inspect metrics consistently without risk of viewing partially-updated statistics or mutating the collector's internal state.

### Collector Metrics

The collector captures the following primary metrics at the engine level:

| Metric | Description |
|---|---|
| `RequestsSent` | Total number of HTTP requests issued by workers |
| `ResponsesReceived` | Total number of HTTP responses received from target servers |
| `RequestsFiltered` | Number of responses filtered out by size, status, or headers |
| `RequestsFailed` | Number of requests that failed due to network errors, timeouts, etc. |
| `RequestsSucceeded` | Number of requests that completed successfully and passed all filters |
| `BytesReceived` | Total volume of data received in response bodies and headers |
| `ActiveWorkers` | Current number of worker goroutines actively processing jobs |
| `QueuedJobs` | Total jobs currently waiting in the frontier queue (manager) |
| `Discovered` | Number of items matching all filters and discovered by the scanner |
| `StartTime` | Timestamp indicating when the collector was initialized |
| `StatusCodes` | Frequency map of discovered responses indexed by HTTP status code |

Additionally, the collector is designed to naturally support future metrics (e.g., `RequestsPerSecond`, `AverageLatency`, `Retries`, `Redirects`, and `BodyInspected`) using dedicated atomic hooks.

---

## Progress Renderer

Searchit implements a modular progress rendering layer designed to periodically display scan state metrics on the terminal.

### Architecture Topology

The rendering architecture operates on a strict uni-directional data pipeline:

```
    [ Workers & Manager ] (Engine Execution Units)
             │
             ▼ (Write-Only updates)
       [ stats.Collector ] (Concurrency-safe Metrics DB)
             │
             ▼ (Snapshot() pull)
       [ stats.Snapshot ]  (Consistent Value Copy)
             │
             ▼ (Read-Only consume)
       [ progress.Manager ] (Periodic Refresher Loop)
             │
             ▼ (Call Render())
      [ progress.Renderer ] (Stateless Formatter Interface)
             │
             ▼ (Print)
        Stdout / TUI
```

### Complete Decoupling of Rendering

Rendering is completely decoupled from the scan engine for the following reasons:
1. **Separation of Concerns**: The execution pipeline (HTTP client, filters, and wordlist processing) should never care about formatting, ANSI codes, or frame-rate ticks. Decoupling ensures core engine modules remain highly testable and free of terminal-specific logic.
2. **Performance Isolation**: The engine performs lock-free metric updates. If a renderer blocks (e.g., due to a slow terminal flush or standard output redirection), it only blocks the background progress manager thread, never the HTTP workers.
3. **Future Extension**: Because the `Renderer` is a simple stateless interface receiving an immutable snapshot, adding future presentation layers (such as full interactive terminal TUIs, JSON serialization endpoints, status widgets, or distributed dashboards) requires zero changes to the underlying statistics engine or scanner.

---

## Live Progress Rendering

Searchit utilizes an active ANSI-based live progress renderer to redraw status blocks in-place on the terminal.

### In-Place Redrawing

Instead of printing static text blocks that scroll and grow the terminal history indefinitely, the `ANSIRenderer` performs dynamic cursor repositioning:
- **ANSI Escape Sequences**: On each tick, the renderer moves the cursor up by the exact count of lines printed during the previous render frame using the `\033[<N>A` (cursor up) sequence.
- **Line Clearing**: Every line output is prefixed with the `\033[K` (clear from cursor to end of line) sequence. This clears any trailing residues from previous longer outputs, completely preventing screen tearing.
- **Constant Block Height**: The renderer uses a fixed-size slot allocations scheme for printing recent discoveries. When fewer than 10 items are logged, empty space is cleared and printed, maintaining a steady, jitter-free height.

### Terminal Compatibility

By using only basic ANSI X3.64 control codes (cursor movement, line clearing, and cursor show/hide visibility), Searchit remains highly compatible with standard terminal emulators across Linux, macOS, and Windows (via Virtual Terminal Processing support). It avoids full-screen alternate buffer configurations, meaning standard output history preceding the scan remains preserved and scrollable.

### Cursor Lifecycle

To provide a premium visual experience, the cursor is hidden (`\033[?25l`) while the periodic rendering loop is active. Upon scan completion (or cancellation/exit via context cancellation), a final snapshot is rendered and the cursor is restored (`\033[?25h`) cleanly via a deferred lifecycle invocation.

---

## Profile-based Scanning

Searchit supports composite runtime configuration overrides through the `--profile` flag.

### Merge Order & Precedence

Scan configurations are built progressively layers by layer, ordered from lowest priority (least specific) to highest priority (most specific):

```
     1. Hardcoded Default Configuration (config.Default())
                            ↓
     2. Profile Overlays (Applied left-to-right, e.g. base -> php -> bugbounty)
                            ↓
     3. Explicit CLI Flags (Highest priority override, e.g. --threads 8)
```

CLI flags always win over all profiles. Conflicts between profiles are resolved in favor of the rightmost profile (last-write-wins).

### Overlay Pipeline Lifecycle

For each specified profile, the configuration manager performs the following operations in a strict, isolated transactional sequence:

1. **Load**: Look up and read the profile YAML file from the user profile directory (`~/.config/searchit/profiles/`) or embedded defaults.
2. **Decode**: Parse the header to determine the schema version and decode the raw document into a generic `Profile` representation.
3. **Validate**:
   - Run generic validation rules (e.g. valid namespace structure and schema format).
   - Dispatch to tool-specific validator implementations (e.g. scan validator ensuring threads $\ge$ 1).
4. **Decode into Overlay**: Map the runtime `Profile` contents to a tool-specific configuration representation (`scan.Overlay`).
5. **Apply**: Merge the overlay fields onto the active runner's configuration.

If any stage of this sequence fails for any profile, the CLI aborts execution immediately with a non-zero exit status, printing the failure reason to `stderr`.

### Practical Examples

Applying multiple profiles sequentially overlays custom rules over standard baselines:

```bash
# Basic quick scan
searchit scan -u https://example.com --profile quick

# Multi-layered overlay:
# 1. Base applies general rules (e.g. threads: 32)
# 2. PHP overlays PHP-specific filters (e.g. exclude-status: 404,500)
# 3. Bugbounty overlays extra-aggressive thresholds (e.g. threads: 128)
# 4. CLI --threads 8 overrides all profile configurations
searchit scan -u https://example.com --profile base --profile php --profile bugbounty --threads 8
```

---

## Profile Metadata

Searchit profiles support rich metadata fields to enhance discovery and prepare for future smart profile selections:

| Field | Type | Description |
|---|---|---|
| `author` | `string` | The developer or organization that created the profile. |
| `tags` | `[]string` | Searchable categorization tags (e.g., `wordpress`, `php`, `cms`). |
| `homepage` | `string` | Canonical website URL or GitHub repository containing the profile. |
| `license` | `string` | Software license descriptor (e.g. `MIT`, `GPL-3.0`). |
| `created` | `string` | ISO-8601 calendar date or timestamp when the profile was initialized. |
| `updated` | `string` | ISO-8601 calendar date or timestamp of the latest modifications. |
| `depends` | `[]string` | Names of parent profiles from which configuration fields are overlaid. *Note: Reserved for future schema inheritance models.* |
| `experimental` | `bool` | Flags unstable or draft configurations (defaults to `false`). |

---

## Profile Dependency Resolution

Searchit profiles support declaring dependencies on other profiles using the `depends` metadata field.

### Dependency Resolution Graph & Sorting

When loading profiles, Searchit constructs a directed dependency graph and resolves it using a topological sorting algorithm (via Depth-First Search with visited node tracking):
- **Dependencies First, Profile Last**: The resolver determines the exact application order where all dependencies are resolved and queued before the target profile.
- **Duplicate Elimination**: If multiple profiles in the graph depend on the same underlying profile (e.g. both `wordpress` and `cms` depend on `php`), that dependency is resolved and applied exactly once in the order.
- **Cycle Detection**: The resolver tracks visiting states to detect cyclic dependencies (e.g. profile A depends on B, B depends on C, and C depends on A). If a cycle is detected, the resolver immediately aborts and returns a detailed `cyclic profile dependency detected` error containing the full chain of circular dependencies.

### Tool Namespace Inheriting

Dependencies are specified as namespaces:
- If a dependency name contains a slash (e.g. `scan/base`), it is resolved as is.
- If a dependency name does not contain a slash (e.g. `base`), it inherits the parent profile's tool/namespace prefix (e.g. if the parent profile is named `scan/wordpress`, `base` is resolved to `scan/base`).

This decoupled design prepares the profile manager for future smart profile loading where profiles across different tools (`scan`, `fuzz`, `subdomain`) can resolve their configuration pipelines cleanly without tight coupling to the scan engine.













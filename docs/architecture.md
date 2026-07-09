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







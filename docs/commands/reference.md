# Command Reference

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

Searchit is managed via a single unified CLI binary. This page describes all available subcommands and flags.

## `searchit scan`

Performs web content discovery scans to find directories, files, and application endpoints.

### Flags
- `-u, --url <string>`: Target URL(s), comma-separated.
- `--url-file <string>`: File path containing a list of target URLs (one per line).
- `-w, --wordlist <string>`: Wordlist path. Uses the embedded wordlist when no wordlist is specified.
- `-r, --recursive`: Enable recursive directory scanning.
- `-d, --max-depth <uint16>`: Maximum recursion depth (default: `3`). Requires `-r, --recursive` to be enabled.
- `-s, --strategy <bfs|dfs>`: Traversal strategy (default: `bfs`). Requires `-r, --recursive` to be enabled.
- `-t, --threads <int>`: Concurrency worker pool size (default: `32`).
- `--timeout <duration>`: HTTP request timeout (default: `10s`). Supported units: `s`, `ms`, etc.
- `--connect-timeout <duration>`: TCP connect timeout (default: `3s`).
- `--delay <duration>`: Delay between requests per worker (e.g. `100ms`).
- `--rate <float>`: Request rate limit in requests per second.
- `-x, --exclude-status <string>`: Comma-separated status codes to exclude (default: `404`).
- `--recurse-on <string>`: Status filters to trigger recursion (default: `200,301,302,403`). Requires `-r, --recursive` to be enabled.
- `--normalize-paths`: Normalize relative segments in paths (e.g. `././admin` -> `admin`).
- `--collapse-slashes`: Collapse consecutive slashes (e.g. `admin////api` -> `admin/api`).
- `--include-size <string>`: Content length sizes to include (e.g. `100-200,512`).
- `--exclude-size <string>`: Content length sizes to exclude (e.g. `0,123`).
- `-H, --include-header <strings>`: HTTP headers to include (e.g. `Server=nginx`). May be specified multiple times. Header value matching is case-insensitive.
- `--exclude-header <strings>`: HTTP headers to exclude (e.g. `Content-Type=text/plain`). May be specified multiple times. Header value matching is case-insensitive.
- `-o, --output <text|json|ndjson>`: Output format (default: `text`).
- `-q, --quiet`: Print only discovered URLs in text mode.
- `--progress`: Enable the interactive live progress display.

## `searchit profile`

Manages configurations and built-in profiles.

### `searchit profile list`
Lists all discoverable built-in and user-defined profiles.

### `searchit profile show <profile>`
Prints the raw YAML configuration of a profile.

### `searchit profile create <name>`
Creates a new profile skeleton in the user config directory.

### `searchit profile validate <profile_name|file_path>`
Performs syntax and tool-specific validation on a profile name or local YAML file path.

### `searchit profile graph <profile>`
Displays a Unicode ASCII dependency tree visualization.

### `searchit profile explain <profile>`
Displays target metadata, depends chain, applied order, final config, and override history.

### `searchit profile edit <profile>`
Safely opens a profile in the system editor with out-of-band validation.

## `searchit version`

Outputs detailed version, commit SHA, and build timestamp.

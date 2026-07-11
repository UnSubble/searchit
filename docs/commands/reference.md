# Command Reference

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

Searchit is managed via a single unified CLI binary. This page describes all available subcommands and flags.

## `searchit scan`

Performs web content discovery scans to find directories, files, and application endpoints.

### Flags

| Flag | Type | Default | Description | Dependencies |
|------|------|---------|-------------|--------------|
| `-u`, `--url` | string | — | Target URL to scan. | **Required** |
| `-w`, `--wordlist` | string | Embedded wordlist | Path to a custom wordlist file. If omitted, the embedded wordlist is used. | — |
| `-t`, `--threads` | int | `32` | Number of concurrent worker goroutines. | — |
| `-r`, `--recursive` | bool | `false` | Enable recursive directory scanning. | — |
| `-R`, `--max-depth` | int | `3` | Maximum recursion depth. | Requires `-r` |
| `--deep-recursive` | bool | `false` | Continue recursion until the maximum depth is reached. | Requires `-r` |
| `--force-recursive` | bool | `false` | Force recursion even on normally non-recursive responses. | Requires `-r` |
| `--strategy` | string | `bfs` | Recursion strategy (`bfs` or `dfs`). | Requires `-r` |
| `-x`, `--exclude-status` | string | — | Exclude one or more HTTP status codes (supports ranges and wildcards). | — |
| `-i`, `--include-status` | string | — | Only display matching status codes. | — |
| `--include-length` | string | — | Filter by response body size. | — |
| `--exclude-length` | string | — | Exclude responses by body size. | — |
| `--timeout` | duration | `10s` | HTTP request timeout. | — |
| `--retries` | int | `0` | Retry failed requests. | — |
| `--user-agent` | string | Searchit default | Override the HTTP User-Agent header. | — |
| `-H`, `--header` | string array | — | Add custom HTTP headers. May be specified multiple times. | — |
| `-X`, `--method` | string | `GET` | HTTP method to use for requests. | — |
| `--follow-redirects` | bool | `false` | Follow HTTP redirects. | — |
| `--profiles` | string | — | Load one or more predefined scan profiles. | — |
| `-o`, `--output` | string | `text` | Output format (`text`, `json`, `ndjson`). | — |
| `--output-file` | string | — | Write results to a file instead of stdout. | — |
| `--no-progress` | bool | `false` | Disable the interactive progress dashboard. | — |
| `--no-color` | bool | `false` | Disable ANSI colors. | — |
| `-v`, `--verbose` | bool | `false` | Enable verbose logging. | — |
| `-q`, `--quiet` | bool | `false` | Suppress non-essential output. | — |

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
- `-q, --quiet`: Print only discovered URLs in text mode. Also suppresses the automatic progress display.
- `--no-progress`: Disable the live progress display. Progress is enabled automatically when stdout is an interactive terminal; use this flag to suppress it (e.g. in scripts or CI).

## `searchit profile`

Manages configurations and built-in profiles.

### `searchit profile list`
Lists all discoverable built-in and user-defined profiles.

![Profile list output](../../assets/screenshots/searchit%20profile%20list.png)

### `searchit profile show <profile>`
Prints the raw YAML configuration of a profile.

![Profile show YAML output](../../assets/screenshots/searchit%20profile%20show%20scan%E2%81%84wordpress.png)

### `searchit profile create <name>`
Creates a new profile skeleton in the user config directory.

### `searchit profile validate <profile_name|file_path>`
Performs syntax and tool-specific validation on a profile name or local YAML file path.

![Profile validate output](../../assets/screenshots/searchit%20profile%20validate%20scan%E2%81%84wordpress.png)

### `searchit profile graph <profile>`
Displays a Unicode ASCII dependency tree visualization.

![Profile dependency graph](../../assets/screenshots/searchit%20profile%20graph%20scan%E2%81%84wordpress.png)

### `searchit profile explain <profile>`
Displays target metadata, depends chain, applied order, final config, and override history.

![Profile explain metadata](../../assets/screenshots/searchit%20profile%20explain%20scan%E2%81%84wordpress%201.png)
![Profile explain overrides](../../assets/screenshots/searchit%20profile%20explain%20scan%E2%81%84wordpress%202.png)

### `searchit profile edit <profile>`
Safely opens a profile in the system editor with out-of-band validation.

## `searchit version`

Outputs detailed version, commit SHA, and build timestamp.

![Version output](../../assets/screenshots/searchit%20version.png)

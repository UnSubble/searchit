# Profiles Guide

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

Profiles are reusable YAML configurations that bundle scanning settings and metadata.

## Structure

A profile includes metadata and a tool-specific configuration block:
```yaml
schema: 1
name: scan/wordpress
tool: scan
description: WordPress optimized profile
author: searchit
license: MIT
homepage: https://github.com/unsubble/searchit
tags:
  - wordpress
depends:
  - php
config:
  threads: 16
  timeout: 15s
  exclude-status: "404,403,500"
```

## Topological Dependencies & Namespaces

Profiles declare their dependencies using the `depends` field. Searchit resolves dependencies topologically (dependencies are resolved first, target last).

### Relative Namespace Resolution
When a profile name does not contain a slash (`/`), the resolver automatically inherits the namespace of its parent profile. For example, if `scan/wordpress` depends on `php`, the resolver resolves `php` to `scan/php`.
If the parent profile does not have a namespace (e.g. it is just `myprofile`), dependencies without a slash are resolved as-is (e.g. `base` resolves to `base`).

### Fuzzy Base-Name Lookup
When invoking profiles via the CLI (e.g. `searchit scan -u <url> --profile wordpress`), if the specified name does not contain a slash, Searchit performs a fuzzy lookup. It searches both user and built-in profiles by their base names.
- If a unique match is found (e.g. `scan/wordpress`), it is loaded.
- If multiple matches are found across different namespaces, the execution terminates with an ambiguity error showing all candidate matches.

## Configuration Keys Reference

The `config` block supports the following keys:

- `wordlist`: String path to a custom wordlist file.
- `threads`: Integer concurrent workers (e.g., `16`).
- `timeout`: Duration string for request timeout (e.g., `15s`, `500ms`).
- `connect-timeout`: Duration string for TCP connection timeout (e.g., `3s`).
- `recursive`: Boolean (e.g., `true` or `false`).
- `max-depth`: Integer maximum recursion depth (e.g., `3`).
- `strategy`: Traversal strategy (`bfs` or `dfs`).
- `delay`: Duration string for delay between requests (e.g., `100ms`).
- `rate`: Float requests per second limit (e.g., `50.0`).
- `output`: String format (`text`, `json`, `ndjson`).
- `quiet`: Boolean to print only discovered resources.
- `normalize-paths`: Boolean to normalize segments.
- `collapse-slashes`: Boolean to collapse multiple slashes.
- `exclude-status`: Comma-separated list of HTTP status codes to exclude (e.g., `"404,500"`).
- `recurse-on`: Comma-separated list of status codes to trigger recursion on (e.g., `"200,301,302"`).
- `include-size` / `exclude-size`: Size ranges to filter by (e.g., `"100-200,512"`).
- `include-header` / `include-headers`: List of header constraints to match (e.g., `["Server=nginx"]`). Case-insensitive header value matching is supported.
- `exclude-header` / `exclude-headers`: List of header constraints to exclude (e.g., `["Content-Type=text/plain"]`). Case-insensitive header value matching is supported.

## Operations

### Creating a Profile
Create a skeleton profile in user configuration:
```bash
searchit profile create scan/myprofile
```

### Validating a Profile
Validate generic metadata and tool-specific parameters using a profile name or local file path:
```bash
searchit profile validate scan/myprofile
searchit profile validate ./myprofile.yaml
```

### Visualizing Dependency Tree
```bash
searchit profile graph scan/wordpress
```

### Explaining Override Merges
Analyze exactly how settings are overridden across the dependency chain to trace their configuration origin and override history:
```bash
searchit profile explain scan/wordpress
```

### Safe Editing
Open the profile in the default system editor (e.g. `nano` or `vim`). Changes are saved to a temp file first, and only replace the target if validation succeeds:
```bash
searchit profile edit scan/myprofile
```

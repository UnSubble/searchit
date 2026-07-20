# Searchit

Searchit is a concurrent, extensible, and profile-based web content discovery tool. It allows developers and security researchers to discover web content such as directories, files, and application endpoints on web servers.

[![Release](https://img.shields.io/github/v/release/unsubble/searchit)](https://github.com/unsubble/searchit/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/unsubble/searchit/ci.yml?branch=main&label=CI)](https://github.com/unsubble/searchit/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/unsubble/searchit)](go.mod)
[![License](https://img.shields.io/github/license/unsubble/searchit)](LICENSE)

Searchit provides a concurrent scanning engine, topological profile dependency resolution, interactive console progress controls, and a structured output system.

![Live interactive ANSI progress dashboard](assets/screenshots/searchit%20scan%20-u%20example.com.png)

## Core Features
- **Concurrent scan engine**: Configurable worker pool and connection reuse policy.
- **Recursive scanning**: BFS and DFS traversal strategies with configurable recursion depth.
- **Filtering**: Filters responses by status code, body size, and custom headers.
- **Profiles**: Reusable scan configurations supporting inheritance/dependency resolution, creation, validation, visualization, and interactive editing.
- **Interactive TUI**: Live progress display enabled automatically in interactive terminals. Use `--no-progress` to suppress. Console keyboard controls available during scans.
- **Output Formats**: Supports text, JSON, and NDJSON output formats, with an optional quiet text mode.

## Installation
Build from source (requires Go 1.24+):

Using the Makefile (recommended):
```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
make build
```

This compiles the binary to `bin/searchit`. To install it globally to your `GOBIN` path:
```bash
make install
```

Or build manually:
```bash
go build -o bin/searchit .
```

You can run the compiled binary directly:
```bash
./bin/searchit --help
```

## Quick Start
Scan a target with default settings using the embedded wordlist:
```bash
searchit scan -u https://example.com
```

The live progress display is enabled automatically when running in an interactive terminal.
To suppress it, pass `--no-progress`.

![Default text output of a completed scan](assets/screenshots/searchit%20scan%20-u%20example.com.png)

Scan using a built-in profile:
```bash
searchit scan -u https://example.com --profile wordpress
```

## Documentation
Complete guides and documentation are available in the `docs/` directory:
- [Getting Started Guide](docs/getting-started.md)
- [Command Reference](docs/commands/reference.md)
- [Profiles Guide](docs/profiles/guide.md)
- [Scan Configuration](docs/scanning/config.md)
- [Recursion & Determinism Hardening](docs/scanning/recursion.md)
- [Architecture & Technical Design](docs/architecture/details.md)
- [Practical Examples](docs/examples/scenarios.md)
- [Development & Contribution Standards](docs/development/standards.md)
- [Project Roadmap](ROADMAP.md)

## License
Searchit is licensed under the [MIT License](LICENSE).

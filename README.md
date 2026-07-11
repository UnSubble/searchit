# Searchit

Searchit is a concurrent, extensible, and profile-based web content discovery tool. It allows developers and security researchers to discover web content such as directories, files, and application endpoints on web servers.

Searchit provides a concurrent scanning engine, topological profile dependency resolution, interactive console progress controls, and a structured output system.

## Core Features
- **Concurrent scan engine**: Configurable worker pool and connection reuse policy.
- **Recursive scanning**: BFS and DFS traversal strategies with configurable recursion depth.
- **Filtering**: Filters responses by status code, body size, and custom headers.
- **Profiles**: Reusable scan configurations supporting inheritance/dependency resolution, creation, validation, visualization, and interactive editing.
- **Interactive TUI**: Interactive live progress display and console commands for real-time controls.
- **Output Formats**: Supports text, JSON, and NDJSON output formats, with an optional quiet text mode.

## Installation
Build from source (requires Go 1.26+):

Using the build script (embeds version details):
```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
./build.sh
```

Or build using standard Go toolchain directly:
```bash
go build ./...
```

The compiled binary is written to `dist/searchit` (when using `./build.sh`) or standard Go locations. You can run it directly:
```bash
./dist/searchit --help
```
Or move it to a directory in your system `PATH` (e.g., `/usr/local/bin`):
```bash
sudo cp dist/searchit /usr/local/bin/
```

## Quick Start
Scan a target with default settings using the embedded wordlist:
```bash
searchit scan -u https://example.com
```

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
- [Architecture & Technical Design](docs/architecture/details.md)
- [Practical Examples](docs/examples/scenarios.md)
- [Development & Contribution Standards](docs/development/standards.md)
- [Project Roadmap](ROADMAP.md)

## License
Searchit is licensed under the [MIT License](LICENSE).

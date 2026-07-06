![Version](https://img.shields.io/github/v/release/UnSubble/searchit?include_prereleases)
![Go Version](https://img.shields.io/github/go-mod/go-version/UnSubble/searchit)
![License](https://img.shields.io/github/license/UnSubble/searchit)
![Build](https://github.com/UnSubble/searchit/actions/workflows/ci.yml/badge.svg)
![Coverage](https://img.shields.io/badge/coverage-90%25-brightgreen)

# Searchit

Fast, extensible, and technology-aware web content discovery tool inspired by [dirsearch](https://github.com/maurosoria/dirsearch) and [ffuf](https://github.com/ffuf/ffuf).

## Goals

- Simple UX similar to dirsearch.
- Performance comparable to ffuf.
- Technology-aware enumeration.
- Profiles and reusable workflows.
- Smart mutation generation.
- Recursive scanning.

## Philosophy

Searchit prefers clean abstractions and incremental development. Features are added only when they solve real problems and avoid premature complexity.

## Release

The current version is **v0.2.0-alpha**.

This pre-release focuses on usability, filtering, performance controls, and stability. While the CLI is becoming more complete, APIs and behavior may still evolve before the first stable release.

## Current Features

### Scanning

- Concurrent worker pool
- Recursive scanning
- BFS / DFS traversal
- Configurable recursion policies
- Multiple target support
- URL file support
- Embedded wordlists
- File wordlists

### Filtering

- Status filtering
- Header filtering
- Content-Length filtering
- Path normalization
- Slash collapsing

### Performance

- Configurable threads
- HTTP connection pooling
- Request timeout
- Connection timeout
- Request delay
- Global rate limiting

### Output

- Human-readable text output
- Quiet mode
- JSON output
- NDJSON output

### Quality

- Cross-platform builds
- Race detector clean
- Native fuzz tests
- Golden output tests
- High test coverage

## Roadmap

### v0.3.0-beta

- Profile-based scanning
- Progress UI
- Interactive controls
- Better terminal output

### v0.4.0-beta

- Smart scanning
- Technology detection
- Adaptive profiles
- Extension-aware mutations

### v0.5.0-beta

- Generic fuzzing engine
- Header fuzzing
- Parameter fuzzing
- Request templates

## Building

```bash
./build.sh
```

Binaries for supported platforms (Linux, Windows, macOS) are written to `dist/`. The build script embeds version metadata via linker flags.

## Testing

```bash
go test ./...
```

## Quick Start

Scan a target:

```bash
searchit scan -u https://example.com
```

Recursive scan:

```bash
searchit scan \
    -u https://example.com \
    -r \
    --max-depth 3
```

Multiple targets:

```bash
searchit scan \
    -u https://a.com,https://b.com
```

Targets from file:

```bash
searchit scan \
    --url-file targets.txt
```

JSON output:

```bash
searchit scan \
    -u https://example.com \
    --output json
```

Rate limiting:

```bash
searchit scan \
    -u https://example.com \
    --rate 50 \
    --delay 100ms
```

## Development Principles

- Keep abstractions minimal.
- Prefer composition over inheritance.
- Avoid premature optimization.
- Avoid premature generalization.
- Keep the engine independent from CLI frameworks.
- Maintain idiomatic Go code.

## License

MIT © 2026 Ismail KULAK. See [LICENSE](LICENSE).

---

> **Disclaimer:** Searchit is intended for authorized security testing and research purposes only.

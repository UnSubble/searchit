# Searchit

Fast and smart web content discovery tool inspired by [dirsearch](https://github.com/maurosoria/dirsearch) and [ffuf](https://github.com/ffuf/ffuf).

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

The current version is `v0.1.0-alpha`. This is an experimental release intended for early adopters and benchmarking.

## Current Features

- Concurrent scanning
- Embedded wordlists
- File wordlists
- Status filtering
- Recursive scanning
- BFS traversal
- DFS traversal
- Configurable depth
- Configurable threads
- HTTP connection pooling

## Roadmap

- Output abstraction
- JSON output
- Header filtering
- Content-length filtering
- Mutation engine
- Progress indicators
- Advanced recursion policies

## Building

```bash
./build.sh
```

Binaries for supported platforms (Linux, Windows, macOS) are written to `dist/`. The build script embeds version metadata via linker flags.

## Testing

```bash
go test ./...
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

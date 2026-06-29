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

## Status

The project is in early development. The foundations — configuration, HTTP status matching, and application context — are in place. The scanning engine is not yet implemented.

## Planned Features

- High-performance concurrent engine
- Recursive scanning
- Profile system
- Technology detection
- Smart extension filtering
- Mutation engine
- JSON output
- HTTP/2 support
- Plugin architecture

## Building

```bash

./build.sh
```

The binary is written to `dist/searchit`. The build embeds version, commit, and date via linker flags.

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

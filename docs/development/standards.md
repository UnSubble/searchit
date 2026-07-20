# Development & Contribution Standards

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](standards.md) | [Roadmap](../../ROADMAP.md)

---

## Quality Gates

Before submitting any code changes, ensure they pass all quality checks locally (requires Go 1.22+):
```bash
make build
make audit
make test
make race
make chaos
make coverage
```

## Testing Philosophy
- **Unit Tests**: Place tests in the package directory they test.
- **Fuzzing**: Write native Go fuzz tests for input parsers.
- **Race Detector**: Always run test suites under the race detector (`-race`).

## Release Process

You can compile optimized release binaries cross-compiled for Linux, macOS, and Windows using:
```bash
make release
```
This writes the build artifacts with linker flags (injecting Name, Version, Commit SHA, and build timestamp metadata) to the `dist/` directory.

Alternatively, you can compile the package locally using:
```bash
make build
```

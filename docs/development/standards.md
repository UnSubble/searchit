# Development & Contribution Standards

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](standards.md) | [Roadmap](../../ROADMAP.md)

---

## Quality Gates

Before submitting any code changes, ensure they pass all quality checks locally (requires Go 1.26+):
```bash
gofmt -w .
go mod tidy
golangci-lint run # Or run: make lint
go test ./...
go test -race ./...
go test -cover ./...
go build ./...
```

## Testing Philosophy
- **Unit Tests**: Place tests in the package directory they test.
- **Fuzzing**: Write native Go fuzz tests for input parsers.
- **Race Detector**: Always run test suites under the race detector (`-race`).

## Release Process

You can compile version-aware release binaries using the custom build script:
```bash
./build.sh
```
This builds binaries with Go's linker flags (`-ldflags`) to inject Name, Version, Commit SHA, and build timestamp metadata into the `dist/` directory.

Alternatively, you can compile the package directly using Go standard build tools:
```bash
go build ./...
```

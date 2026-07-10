# Contributing

Contributions are welcome. Please read this document before opening an issue or pull request.

## Development Setup

**Requirements:** Go 1.26.4+, Git

```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
./build.sh
go test ./...
```

## Coding Standards

- Follow idiomatic Go conventions.
- Run `gofmt` before committing.
- Run `staticcheck` before opening a PR.
- Prefer explicit code over clever abstractions.
- Do not introduce unnecessary interfaces.
- Keep packages focused and cohesive.
- Avoid global state when possible.

## Pull Requests

Every pull request must pass the following checks before being merged:

1. **gofmt** (checks formatting)
2. **staticcheck** (checks static analysis)
3. **go test** (runs unit/integration tests)
4. **go test -race** (verifies concurrency safety)
5. **go test -cover** (checks test coverage)
6. **go build** (ensures successful builds)

Additionally:
- New behavior includes tests.
- Documentation is updated if necessary.
- No unrelated refactoring in feature PRs.
- Commit messages are meaningful.

## Philosophy

Searchit values **simplicity**, **performance**, **maintainability**, and **incremental evolution**. Avoid adding abstractions before they solve a concrete problem.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

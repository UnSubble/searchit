# Contributing

Contributions are welcome. Please read this document before opening an issue or pull request.

## Development Setup

**Requirements:** Go 1.25+, Git

```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
./build.sh
go test ./...
```

## Coding Standards

- Follow idiomatic Go conventions.
- Run `gofmt` before committing.
- Run `go vet` before opening a PR.
- Prefer explicit code over clever abstractions.
- Do not introduce unnecessary interfaces.
- Keep packages focused and cohesive.
- Avoid global state when possible.

## Pull Requests

- Tests pass (`go test ./...`).
- New behavior includes tests.
- Documentation is updated if necessary.
- No unrelated refactoring in feature PRs.
- Commit messages are meaningful.

## Philosophy

Searchit values **simplicity**, **performance**, **maintainability**, and **incremental evolution**. Avoid adding abstractions before they solve a concrete problem.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

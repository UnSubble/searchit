# Contributing to Searchit

Thank you for your interest in contributing to Searchit. Please review these guidelines before submitting issues or pull requests.

## Development Setup

**Requirements**: Go 1.26+, Git

```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
./build.sh
go test ./...
```

## Coding Standards
- Follow standard Go formatting conventions (run `gofmt` before committing).
- Run linter on all packages (`golangci-lint run` or `make lint`) to ensure there are no static analysis warnings.
- Keep packages focused and avoid circular dependencies.
- Avoid introducing unnecessary interfaces or global state.

## Pull Request Checklist
Every pull request must pass the following local validation pipeline:
1. `gofmt -w .`
2. `go mod tidy`
3. `golangci-lint run` (or `make lint`)
4. `go test ./...`
5. `go test -race ./...`
6. `go test -cover ./...`
7. `go build ./...`

## License
By contributing to Searchit, you agree that your contributions will be licensed under the MIT License.

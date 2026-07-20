# Contributing to Searchit

Thank you for your interest in contributing to Searchit. Please review these guidelines before submitting issues or pull requests.

## Development Setup

**Requirements**: Go 1.22+, Git

```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
make build
make test
```

## Coding Standards
- Follow standard Go formatting conventions (run `gofmt` before committing).
- Run the code audit tools (`make audit`) to ensure there are no static analysis warnings or vet errors.
- Keep packages focused and avoid circular dependencies.
- Avoid introducing unnecessary interfaces or global state.

## Pull Request Checklist
Every pull request must pass the following local validation pipeline:
1. `gofmt -w .`
2. `go mod tidy`
3. `make audit`
4. `make test`
5. `make race`
6. `make chaos`
7. `make coverage`
8. `make build`

## License
By contributing to Searchit, you agree that your contributions will be licensed under the MIT License.

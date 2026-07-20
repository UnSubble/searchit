# Getting Started

[Index](../README.md) | [Getting Started](getting-started.md) | [Command Reference](commands/reference.md) | [Profiles Guide](profiles/guide.md) | [Scanning Guide](scanning/config.md) | [Architecture](architecture/details.md) | [Standards](development/standards.md) | [Roadmap](../ROADMAP.md)

---

Welcome to Searchit, a concurrent, extensible, and profile-based web content discovery tool designed to find directories, files, and application endpoints.

## Installation

Searchit requires Go 1.24 or higher.

To compile from source using the Makefile (recommended):
```bash
git clone https://github.com/unsubble/searchit.git
cd searchit
make build
```
The compiled binary will be placed at `bin/searchit`. You can run it locally as `./bin/searchit` or install it globally to your `GOBIN` path using `make install`.

Alternatively, you can compile the codebase directly using standard Go tooling:
```bash
go build -o bin/searchit .
```

## Quick Start

To verify installation, print the help menu:
```bash
searchit --help
```

![Help output](../assets/screenshots/searchit%20--help.png)

To run a basic scan against a target using the built-in embedded wordlist:
```bash
searchit scan -u https://example.com
```

To run a scan with recursive directory crawling:
```bash
searchit scan -u https://example.com -r --max-depth 3
```

![Recursive scan progress](../assets/screenshots/searchit%20scan%20-u%20example.com%20-r%20--max-depth%203.png)

To scan with a custom wordlist file:
```bash
searchit scan -u https://example.com -w my_wordlist.txt
```

To check available configurations and built-in profiles:
```bash
searchit profile list
```

![Profile list output](../assets/screenshots/searchit%20profile%20list.png)

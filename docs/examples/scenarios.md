# Practical Examples

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

This page provides common usage examples for Searchit.

## Basic Scan
Scan a single web application:
```bash
searchit scan -u https://example.com
```

## Recursive Scan
Recursively scan up to a depth of 3:
```bash
searchit scan -u https://example.com -r -d 3
```

## Scanning Multiple Targets
Scan two targets concurrently:
```bash
searchit scan -u https://a.com,https://b.com
```

Scan targets from a text file:
```bash
searchit scan --url-file targets.txt
```

## Saving Structured Output
Output results in JSON format:
```bash
searchit scan -u https://example.com -o json
```

Output results in Line-Delimited JSON (NDJSON):
```bash
searchit scan -u https://example.com -o ndjson
```

## Running in Quiet Text Mode
Suppress stdout status updates to print only discovered endpoints:
```bash
searchit scan -u https://example.com -q
```

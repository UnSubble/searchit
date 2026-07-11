# Practical Examples

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

This page provides common usage examples for Searchit.

## Basic Scan
Scan a single web application:
```bash
searchit scan -u https://example.com
```

![Basic scan output](../../assets/screenshots/searchit%20scan%20-u%20example.com.png)

## Recursive Scan
Recursively scan up to a depth of 3:
```bash
searchit scan -u https://example.com -r -d 3
```

![Recursive scan output](../../assets/screenshots/searchit%20scan%20-u%20example.com%20-r%20-d%203.png)

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
searchit scan -u http://localhost:8080 -o json
```

![JSON output example](../../assets/screenshots/searchit%20scan%20-u%20localhost%3A8080%20-o%20json.png)

Output results in Line-Delimited JSON (NDJSON):
```bash
searchit scan -u http://localhost:8080 -o ndjson
```

![NDJSON output example](../../assets/screenshots/searchit%20scan%20-u%20localhost%3A8080%20-o%20ndjson.png)

## Running in Quiet Text Mode
Suppress stdout status updates to print only discovered endpoints:
```bash
searchit scan -u http://localhost:8080 -q
```

![Quiet mode output example](../../assets/screenshots/searchit%20scan%20-u%20localhost%3A8080%20-q.png)

## Disabling Progress
Progress is enabled automatically in interactive terminals. To suppress it explicitly:
```bash
searchit scan -u https://example.com --no-progress
```

Progress is also suppressed automatically when:
- Output is piped or redirected (non-TTY stdout)
- `--quiet` / `-q` is active
- `--output json` or `--output ndjson` is set

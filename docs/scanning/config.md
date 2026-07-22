# Scan Configuration

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](config.md) | [Recursion Guide](recursion.md)

---

## Target-Aware Adaptive Scanning (`--adaptive`)

Searchit features an opt-in Target-Aware Adaptive Engine (`--adaptive`). When enabled, Searchit probes target endpoints for technology signatures (WordPress, Laravel, Express) and metadata files (`robots.txt`, `sitemap.xml`).

```bash
searchit scan -u http://127.0.0.1:8080 -w ~/wordlists/rockyou.txt --adaptive
```

![Adaptive Scanning](../../assets/screenshots/scan_adaptive.png)

- **Candidate Prioritization**: High-relevance paths (score > 50) are pushed to the front of the recursion frontier (`PushFront`).
- **Zero Suppression Guarantee**: Adaptive prioritization **never suppresses or skips candidates**. All paths in your wordlist are scanned.


---

## Statistical Wildcard Detection

Searchit automatically detects and suppresses dynamic wildcard 404 responses (e.g. servers returning HTTP 200 or custom 404 pages for any arbitrary requested path).

- **Modal Signature Analysis**: Tracks `(StatusCode, BodyHash, BodySize)` signatures per `host:depth` key.
- **Automatic Suppression**: Matches subsequent responses against the detected wildcard signature to eliminate false positive discoveries.

```bash
searchit scan -u http://127.0.0.1:8080 -w ~/wordlists/rockyou.txt
```

SCREENSHOT:
    assets/screenshots/scan/wildcard.png

STATUS:
    MISSING

DESCRIPTION:
    Demonstrates automatic modal signature analysis and wildcard response filtering.

---

## Recursion

Searchit supports recursive discovery of subdirectories using two strategies:
- **BFS (Breadth-First Search)**: Scans all directories at the current level before going deeper.
- **DFS (Depth-First Search)**: Crawls as deep as possible along a branch before backtracking.

![BFS vs DFS Traversal Comparison](../../assets/docs/bfs-vs-dfs.png)

Configuring recursion:
```bash
searchit scan -u http://127.0.0.1:8080 -r -d 3 -s bfs
```

---

## Unified Response Filtering & Matching

Filters are applied in a strict, low-cost pipeline to discard unwanted HTTP responses:
- **Status Filter**: Ignore/match HTTP status codes (`--mc 200,302`, `--fc 404,500`).
- **Size Filter**: Include/exclude response body size bounds (`--ms 100-500`, `--fs 0`).
- **Regex Filter**: Match/filter response body content (`--mr "admin"`, `--fr "not found"`).
- **Content-Type Filter**: Match/filter by Content-Type header (`--mt "application/json"`, `--ft "text/html"`).
- **Header Filter**: Match custom HTTP headers case-insensitively using `-H Name=Value` (include) or `--exclude-header Name=Value` (exclude).

![Response Filter Pipeline](../../assets/docs/response-filter-pipeline.png)

---

## Rate Limiting and Delays

To avoid overwhelming target servers, you can limit throughput:
- `--rate <req/s>`: Restrict the maximum request rate.
- `--delay <duration>`: Insert a fixed sleep interval between worker requests (e.g. `--delay 100ms`).

---

## Interactive Live Progress Display & Keyboard Controls

The live progress display is **enabled automatically** whenever stdout is an interactive terminal (TTY). No flag is required.

```bash
searchit scan -u http://127.0.0.1:8080 -w ~/wordlists/rockyou.txt
```

Progress is suppressed automatically when:
- stdout is not a terminal (piped output, redirected to a file, or CI environment)
- `--quiet` / `-q` is active
- `--output json`, `--output ndjson`, `--output csv`, or `--output md` is set

To explicitly disable progress in an interactive terminal, pass `--no-progress`:

```bash
searchit scan -u http://127.0.0.1:8080 -w ~/wordlists/rockyou.txt --no-progress
```

Interactive controls during scans:
- `p`: Force-redraw the progress interface.
- `s`: Print extended statistics.
- `q`: Stop current target.
- `Ctrl+C`: Abort everything.

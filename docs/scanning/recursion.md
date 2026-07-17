# Recursion & Determinism Hardening Guide

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](config.md) | [Recursion Guide](recursion.md) | [Architecture](../architecture/details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

Searchit features a highly performant and hardened recursion subsystem. It is designed to safely, reliably, and deterministically discover nested directory structures on target servers.

## Traversal Strategies

Searchit supports two directory traversal strategies:

- **BFS (Breadth-First Search)**: Explores directories level-by-level (e.g. root paths first, then all nested subdirectories at depth 1, then depth 2). This is the default strategy.
- **DFS (Depth-First Search)**: Prioritizes deeper directory hierarchies first, crawling down a branch as far as possible before backtracking to siblings.

Traversal is enabled using the `-r` or `--recursive` flag. You can set the strategy via `--strategy bfs` or `--strategy dfs`, and enforce a limit using `-R` or `--max-depth <depth>`.

```bash
searchit scan -u https://example.com -r --strategy bfs --max-depth 3
```

---

## Determinism Across Worker Counts

One of Searchit's core design principles is **strict determinism**. 

Regardless of the number of concurrent worker threads allocated via `-t/--threads` (from 1 to 256), a given scan target and configuration **must produce exactly the same result set**.

### What Is Checked for Determinism?

Searchit's hardening test matrix validates and enforces that the following properties match perfectly across all worker counts:
1. **URL count**: The total number of discovered endpoints.
2. **URL ordering**: The canonical sorted list of discovered URLs.
3. **Recursion depth**: The depth at which each URL was discovered.
4. **Duplicates**: Ensuring duplicate suppression works identically under high concurrency.
5. **Output formatters**: Text, JSON, NDJSON, CSV, and Markdown outputs are validated to be character-for-character identical after sorting.
6. **Profile behavior**: Config overrides and dependency merges remain identical.

---

## Benchmark & Hardening Levels

To balance validation depth and execution speed, the testing and benchmarking suites are organized into five levels, controlled by the `BENCHMARK_LEVEL` environment variable.

| Level | Name | Purpose | Target Runtime | Workload / Worker Counts |
|:---:|---|---|:---:|---|
| **1** | **SMOKE** | Commit validation (default) | `< 30s` | Small targets (5 URLs), low workers (1, 4) |
| **2** | **TEST** | Feature implementation | `< 5m` | Medium targets (20 URLs), workers (1, 2, 4, 16) |
| **3** | **HARDENING** | Pre-release validation | `< 15m` | Medium targets (100 URLs), workers (1, 4, 16, 64), timeout checks |
| **4** | **BENCHMARK** | Release benchmarking | `< 60m` | Large targets (500 URLs), workers up to 128, performance metrics |
| **5** | **RIGOROUS** | Major release validation | Unrestricted | Huge targets (1000 URLs), workers up to 256, randomized executions |

To run the suites under a specific level, set the environment variable:

```bash
BENCHMARK_LEVEL=3 go test -v ./internal/recursion/ -run TestHardening
```

Alternatively, use the Make targets which run under the safe default of level 1:

```bash
# Runs fast smoke validation tests
make test

# Runs race detector validation tests
make race

# Runs benchmarks
make benchmark
```

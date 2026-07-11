# Architecture & Technical Design

[Index](../../README.md) | [Getting Started](../getting-started.md) | [Command Reference](../commands/reference.md) | [Profiles Guide](../profiles/guide.md) | [Scanning Guide](../scanning/config.md) | [Architecture](details.md) | [Standards](../development/standards.md) | [Roadmap](../../ROADMAP.md)

---

Searchit is engineered for throughput, concurrency safety, and predictable resource bounds.

## Worker Pool & Orchestration

Searchit utilizes a producer-consumer orchestration model:
- **Producer**: Generates job values representing URLs to scan and sends them to a jobs channel.
- **Worker Pool**: Spawns $N$ concurrent worker goroutines that pull jobs, perform HTTP client exchanges, filter responses, and emit discoveries.
- **Consumer**: Receives discoveries and formats them to the target output.

## Response Pipeline

Every HTTP response passes through an ordered sequence of filters. The cheapest validation steps run first to save resources:
1. **Status Code Check**: Checked first using integer bounds.
2. **Headers Check**: Evaluated against configured rules.
3. **Content-Length Check**: Checked before body reads.
4. **Body Read**: Bounded to at most 4096 bytes, then discarded to preserve predictable memory usage.

## Decoupled Statistics Engine

Statistics collection is lock-free and pre-allocated:
- Atomic variables (`sync/atomic`) are used for simple counters.
- Status code frequency is tracked using a pre-allocated fixed-size array of 1000 counters (indexing codes 0-999). This avoids mutex locks during concurrent increments.

## Interactive Live Progress Display

The `ANSIRenderer` uses terminal ANSI control codes to redraw progress updates. Alt-screen alternate buffers are bypassed to preserve terminal history. Redraw operations run in a background progress loop, completely decoupled from the scanner workers.

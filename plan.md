# cdcviz — plan and status

**Status:** v0.1, complete. A terminal-first inspector for change-data-capture
streams, written in Go with no third-party dependencies.

## Goal

CDC tooling usually shows rows, not the story between rows. The expensive bugs
are ordering and lifecycle bugs: updates before inserts, duplicate deliveries,
resurrections, stale timestamps, and LSNs that jump backwards. `cdcviz` should
make those visible in one screen, from a file or a pipe, with no database
connection and no daemon.

Non-goal: being a streaming platform. `cdcviz` is a debugging and QA tool that
sits at the end of a pipeline.

## Delivered in v0.1

- [x] Normalized event model (`lsn`, `ts`, `op`, `table`, `pk`, `before`,
      `after`) shared by every view.
- [x] Native NDJSON parser plus Debezium Postgres/MySQL envelope support.
- [x] Per-line auto-detection, `--format` override, and `--pk-field` fallback.
- [x] Flexible timestamps (RFC 3339, seconds, ms, µs, ns) and LSNs (int,
      string, Postgres `hi/lo`).
- [x] Ten stream-integrity checks with severity levels.
- [x] Lifecycle timeline with a shared sequence axis and busiest-first rows.
- [x] Per-table summary with totals.
- [x] Single-record history with before/after field changes.
- [x] Aggregate stats with throughput and anomaly rollups, plus JSON output.
- [x] Animated replay with speed and limit controls.
- [x] Deterministic demo scenario that injects eight representative bugs.
- [x] CLI with flags before or after the command, color control, and examples.
- [x] Tests for parsing, detection, stats, every renderer, and the CLI.
- [x] CI (gofmt, vet, race tests, build) and sample NDJSON inputs.

## Architecture decisions

**Normalize early.** Parsers produce one `event.Event`; analysis and rendering
never see a Debezium envelope. Adding a format means adding a parser, not
touching five views.

**Per-line format detection.** CDC exports are often concatenations (a snapshot
followed by streaming), so auto-detection is per line with an explicit override
for pathological inputs.

**Sequence axis for the timeline.** Time axes look authoritative but lie when
clocks are skewed or timestamps are missing. Event position exposes bursts and
gaps honestly; timestamps are shown alongside instead.

**Heuristics with severities.** `update-before-insert` and
`delete-before-insert` are errors because they usually mean a skipped record, but
they are expected when a capture starts mid-history. Severities and a JSON
output keep the tool useful without pretending the checks are proofs.

**In-memory, multi-pass analysis.** Anomaly detection, stats, and all views need
the whole stream. Keeping it in memory keeps the code simple and the tool fast
for its intended size (debugging captures, not billions of events).

**Stdlib-only rendering.** ANSI escapes and `text/tabwriter` cover the layout;
no TUI framework, no terminal capability database, predictable output in pipes.

**Animation only on a terminal.** Replay writes normal lines when piped, so logs
and tests stay diffable. `--speed 0` makes playback instant.

## Anomaly catalogue

| Kind | Severity | Rationale |
| --- | --- | --- |
| `update-before-insert` | error | missing row image or skipped insert |
| `delete-before-insert` | error | delete for a record never observed |
| `out-of-order-lsn` | warning | log position moved backwards |
| `stale-timestamp` | warning | event older than the key's previous event |
| `duplicate-event` | warning | exact redelivery |
| `duplicate-insert` | warning | insert while the key is live |
| `resurrection` | warning | insert after delete |
| `update-after-delete` | warning | change to a deleted key |
| `delete-after-delete` | warning | repeated delete |
| `no-op-update` | info | update with identical before/after |

## Non-goals for v0.1

- No database or Kafka connector; input is NDJSON.
- No transaction grouping or schema registry integration.
- No anomaly allowlisting or per-table rule configuration.
- No windowed/streaming aggregation; everything is loaded first.
- No HTML/PNG export.
- No auth, remote sources, or long-running consumption.

## Roadmap

### v0.2 — pipeline ergonomics
- [ ] `cdcviz tail` for a long-running stdin stream with bounded memory.
- [ ] `--fail-on error|warning|any` for CI gates (with a dedicated exit code).
- [ ] Anomaly allowlist file (`--ignore-kind`, `--ignore-table:pk`).
- [ ] Multiple inputs merged by LSN, for comparing a source and a replica.
- [ ] `--since` / `--until` time filters and `--table` filters.

### v0.3 — more sources and output
- [ ] Postgres logical replication (`pgoutput`) reader behind a build tag.
- [ ] Kafka consumer via a small adapter, still optional.
- [ ] HTML report with the timeline and anomaly list, for sharing.
- [ ] CSV/JSON export of anomalies and per-key histories.
- [ ] Shell completion for bash/zsh/fish.

### v0.4 — deeper analysis
- [ ] Transaction windows (`txid`, begin/commit events).
- [ ] Schema-evolution checks (column appeared/disappeared, type changes).
- [ ] Per-key gap detection for monotonic sequence columns.
- [ ] Latency analysis: source commit time vs. event time when available.
- [ ] Side-by-side comparison of two streams to find divergence at the first hop.

### Quality
- [ ] Fuzz the parsers and auto-detection with malformed envelopes.
- [ ] Golden-file tests for every view at several widths.
- [ ] Benchmarks for 1M-event parse/analyze/render.
- [ ] Property tests for the detector state machine (ordering independence of
      duplicate detection where applicable).
- [ ] Terminal-width detection instead of a fixed default.

## Verification

```sh
make test      # unit, renderer, and CLI integration tests
make race
make vet
make demo      # timeline + tables + anomalies over the built-in scenario
```

The demo scenario is itself a test fixture: `internal/demo` asserts that all
eight injected anomaly kinds are still detected, so the demo cannot silently rot.

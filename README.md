# cdcviz

[![ci](https://github.com/rushikeshg25/cdcviz/actions/workflows/ci.yml/badge.svg)](https://github.com/rushikeshg25/cdcviz/actions/workflows/ci.yml)
[![Go 1.24+](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**A terminal inspector for change-data-capture streams.** `cdcviz` turns
newline-delimited CDC events into lifecycle timelines, per-table summaries,
record histories, and integrity findings — with no database, no daemon, and no
third-party dependencies.

```
$ cdcviz demo
Timeline  17 events | 4 records | 10:00:00.000 -> 10:00:23.000

users/1      +...~.............~....~................................+........~....~.....   7 events
orders/10    ..............+.............~........~......................~...............   5 events
users/2      .........+......................-.............~....+........................   4 events
orders/11    ..........................................-.................................   1 events

Legend:  + insert   ~ update   - delete   . read
```

Most CDC tooling shows you rows. The hard bugs are not in the rows — they are in
the story between them: an update that arrives before its insert, a delete that
is delivered twice, a row that comes back from the dead, an LSN that moves
backwards, or a delete for a row that was never inserted. `cdcviz` is built to
make those visible in one screen.

It is aimed at developers and data engineers debugging replication pipelines,
QA sessions, and incident reviews: pipe in a capture, scan the timeline, open the
suspect record, and get a list of ordering and lifecycle problems without
standing up a database or a UI. It is a local analysis tool, not a production
consumer; see [Limits](#limits).

## Install

Requires Go 1.24+ and no third-party dependencies (Linux, macOS, or Windows).

```sh
go install github.com/rushikeshg25/cdcviz/cmd/cdcviz@latest

# or from a checkout:
go build -o bin/cdcviz ./cmd/cdcviz
./bin/cdcviz demo
```

Or `make build`, `make test`, `make demo`.

## Try it

```sh
# Built-in scenario with eight deliberate integrity problems.
cdcviz demo

# The same scenario as an NDJSON stream (input for the other commands).
cdcviz demo --json > demo.jsonl

cdcviz timeline  --input demo.jsonl
cdcviz tables    --input demo.jsonl
cdcviz anomalies --input demo.jsonl
cdcviz record users 1 --input demo.jsonl
cdcviz replay    --input demo.jsonl --speed 4
cdcviz stats     --input demo.jsonl --json

# A Debezium-style file works without flags; the format is detected per line.
cdcviz tables --input examples/debezium-users.jsonl
```

Input comes from stdin when `--input` is omitted. Each line must contain exactly
one event; trailing data after the JSON object is an error, not a silent
truncation.

```sh
kcat -C -t cdc.users | cdcviz anomalies
```

## Input formats

### Native (smallest useful event)

```json
{"lsn":103,"ts":"2026-01-01T10:00:02Z","op":"u","table":"users","pk":"1",
 "before":{"id":1,"name":"Ada"},"after":{"id":1,"name":"Ada Lovelace"}}
```

- `op` accepts `insert|update|delete|read` and Debezium's `c|u|d|r`.
- `pk` may be omitted if `id`, `pk`, or `key` exists in the row image, or if
  `--pk-field` names the column.
- `ts` accepts RFC 3339 or an epoch number in seconds, milliseconds,
  microseconds, or nanoseconds. Numeric units are inferred from absolute magnitude
  (at least `1e11` = ms, `1e14` = µs, `1e17` = ns). Fractional seconds and
  nanoseconds retain their precision; zero is the Unix epoch, and negative
  values represent dates before it. Use `null` or omit `ts` for unknown time.
- `lsn` accepts a number, a numeric string, or Postgres `hi/lo` hex.

### Debezium

```json
{"payload":{"op":"c","after":{"id":1,"name":"Ada"},
 "source":{"table":"users","lsn":100,"ts_ms":1767261600000}}}
```

Both the Postgres (`source.lsn`) and MySQL (`source.pos` + top-level `key`)
envelopes are supported. `--format native|debezium` disables auto-detection.
MySQL `pos` values are only comparable within one binlog file, so treating them
as a global sequence can produce false `out-of-order-lsn` findings.

## Commands

```
timeline              lifecycle bars for every record
tables                per-table event counts
stats                 aggregate counters (--json)
anomalies             ordering, duplication, and lifecycle problems (--json)
record <table> <pk>   full history of one record
replay                replay events one by one (--speed, --limit)
demo                  run the built-in scenario through the main views
version               print the version
```

```
-i, --input FILE    read from FILE instead of stdin ("-" means stdin)
    --format NAME   auto, native, or debezium
    --pk-field NAME primary key column to derive from row images
    --color WHEN    auto, always, or never
    --width N       timeline width (default 100)
    --max-rows N    timeline rows (default 20)
    --json          JSON output for stats, anomalies, and demo
    --speed N       replay events per second (0 = no delay, default 8)
    --limit N       replay only the first N events
```

Use `--` before record arguments that begin with a dash, for example:
`cdcviz record --input events.jsonl -- users -1`. Unexpected positional
arguments and invalid replay speeds produce a usage error (exit code 2).

`--color auto` is on only for a terminal and honours `NO_COLOR`. `replay`
animates only when writing to a terminal, so piped output stays line-oriented.

## Views

**`record`** shows one row's whole life, including before/after values:

```
$ cdcviz record users 1 --input demo.jsonl
users/1  7 events | 10:00:00.000 -> 10:00:22.000

  #1  10:00:00.000  + INSERT  lsn=100
        email: "ada@example.com", id: 1, name: "Ada"
  #2  10:00:02.000  ~ UPDATE  lsn=103
        name: "Ada" -> "Ada Lovelace"
...
```

**`tables`** is a quick reconciliation table:

```
TABLE   EVENTS  INSERT  UPDATE  DELETE  READ  KEYS  ANOMALIES
orders  6       1       3       1       1     2     2
users   11      4       6       1       0     2     6
TOTAL   17      5       9       2       1     4     8
```

**`stats --json`** is meant for scripts and CI dashboards (output abridged):

```json
{
  "events": 17,
  "keys": 4,
  "ops": {"delete": 2, "insert": 5, "read": 1, "update": 9},
  "first_ts": "2026-01-01T10:00:00Z",
  "last_ts": "2026-01-01T10:00:23Z",
  "duration_seconds": 23,
  "events_per_second": 0.7391304347826086,
  "anomalies": 8,
  "anomalies_by_severity": {"error": 1, "info": 1, "warning": 6}
}
```

## What it checks

| Kind | Severity | Meaning |
| --- | --- | --- |
| `update-before-insert` | error | the stream starts with an update, or an insert was skipped |
| `delete-before-insert` | error | a delete arrived for a row never seen in the stream |
| `out-of-order-lsn` | warning | the log sequence moved backwards |
| `stale-timestamp` | warning | an event is older than the same key's previous event |
| `duplicate-event` | warning | the exact same event was delivered twice |
| `duplicate-insert` | warning | an insert arrived for a key that is already live |
| `resurrection` | warning | a deleted key was inserted again |
| `update-after-delete` | warning | an update arrived for a deleted key |
| `delete-after-delete` | warning | a delete was applied to an already deleted key |
| `no-op-update` | info | an update with no field changes |

Anomalies are heuristics, not proof of corruption. In particular, starting a
stream mid-history will report `update-before-insert` and
`delete-before-insert`; those are often expected for a partial capture.

## How it works

```
NDJSON / Debezium
       |
       v
  format detection -> parser -> normalized event
                                  |      \
                                  |       +--> anomaly detector
                                  v
                            stats aggregation
                                  |
                                  v
                 timeline | tables | record | anomalies | replay
```

The normalized event is the contract: every view and every check works on the
same `{lsn, ts, op, table, pk, before, after}` value, so adding a new input
format does not touch the analysis or rendering code.

Design choices:

- **Sequence axis, not time axis.** The timeline uses event position, so bursts
  and gaps stay visible even when timestamps are missing or skewed.
- **Stdlib only.** Colors, layout, and animation are ANSI/`text/tabwriter`;
  there is no TUI framework to fight.
- **JSON in, text out.** Inputs are NDJSON; `stats` and `anomalies` also emit
  JSON, making the tool composable with `jq` and CI.
- **Load the stream, then analyze.** Events are kept in memory so all views can
  make multiple passes; this is a development/QA tool, not a long-running
  consumer.

## Limits

- There is no database connector: feed it NDJSON from Debezium, Kafka Connect's
  file sink, `pgoutput`-to-JSON tooling, or your own producer.
- The whole input is held in memory; this is aimed at debugging-sized streams
  (thousands to low millions of rows).
- Primary keys must be present or derivable (`pk`, `id`, `key`, or `--pk-field`).
- Transactions are not grouped; each event is treated independently.
- Anomaly rules are stream-local and have no schema or allowlist support.
- LSN gaps and schema changes are not modeled. The detector can see a sequence
  move backwards, but not a record that never arrived.

## Status

v0.1 is complete and usable: both input formats, all views, ten anomaly checks,
JSON output, tests, and CI are in place. CI runs `gofmt`, `go vet`, and
`go test -race`. See [plan.md](plan.md) for the roadmap, including gap detection
and a `--fail-on` exit code for CI.

## Development

```sh
make test    # go test ./...
make race    # go test -race ./...
make vet
make demo
```

```
cmd/cdcviz        CLI and flag parsing
internal/event    normalized event, native/Debezium parsers, streaming decoder
internal/anomaly  integrity detector
internal/stats    per-table and per-operation aggregation
internal/render   timeline, tables, record, anomalies, stats, replay
internal/demo     deterministic scenario used by `cdcviz demo`
```

See [plan.md](plan.md) for milestones and the roadmap.

MIT licensed.

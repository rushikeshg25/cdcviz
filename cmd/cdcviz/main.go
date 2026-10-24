// Command cdcviz inspects change-data-capture streams from the terminal.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/demo"
	"github.com/rushikeshg25/cdcviz/internal/event"
	"github.com/rushikeshg25/cdcviz/internal/render"
	"github.com/rushikeshg25/cdcviz/internal/stats"
)

const version = "0.1.0"

const usage = `cdcviz - inspect change-data-capture streams in the terminal

usage:
  cdcviz [flags] <command> [arguments]

commands:
  timeline              lifecycle bars for every record
  tables                per-table event counts
  stats                 aggregate counters (--json)
  anomalies             ordering, duplication, and lifecycle problems (--json)
  record <table> <pk>   full history of one record
  replay                replay events one by one (--speed, --limit)
  demo                  run the built-in scenario
  version               print the version

input:
  Reads newline-delimited JSON from stdin, or --input FILE.
  Use "cdcviz demo --json" to produce a sample stream.

formats:
  auto       detect native or Debezium per line (default)
  native     {"lsn":1,"ts":"2026-01-01T10:00:00Z","op":"i",
              "table":"users","pk":"1","after":{"id":1}}
  debezium   {"payload":{"op":"c","after":{...},
              "source":{"table":"users","lsn":1,"ts_ms":1767261600000}}}

flags (may appear before or after the command):
  -i, --input FILE    read from FILE instead of stdin ("-" means stdin)
      --format NAME   auto, native, or debezium
      --pk-field NAME primary key column to derive from row images
      --color WHEN    auto, always, or never
      --width N       timeline width (default 100)
      --max-rows N    timeline rows (default 20)
      --json          JSON output for stats, anomalies, and demo
      --speed N       replay events per second (0 = no delay, default 8)
      --limit N       replay only the first N events
      --              treat remaining arguments as literal values
`

type options struct {
	input   string
	format  event.Format
	pkField string
	color   string
	width   int
	maxRows int
	json    bool
	speed   float64
	limit   int
}

var errUsage = errors.New("usage")

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "cdcviz:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, rest, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "cdcviz: %v\n\n%s", err, usage)
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stdout, usage)
		return nil
	}

	cmd, cmdArgs := rest[0], rest[1:]
	switch cmd {
	case "timeline", "tables", "stats", "anomalies", "replay", "demo", "version":
		if len(cmdArgs) != 0 {
			fmt.Fprintf(stderr, "cdcviz: %s takes no arguments\n\n%s", cmd, usage)
			return errUsage
		}
	case "record":
		if len(cmdArgs) != 2 {
			fmt.Fprintf(stderr, "cdcviz: record needs <table> <pk>\n\n%s", usage)
			return errUsage
		}
	}
	theme := render.NewTheme(colorEnabled(opts.color, stdout))

	switch cmd {
	case "timeline":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		render.Timeline(events, stdout, render.TimelineOptions{
			Width: opts.width, MaxRows: opts.maxRows, Theme: theme,
		})
		return nil
	case "tables":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		render.Tables(stats.Compute(events, anomaly.Detect(events)), stdout)
		return nil
	case "stats":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		summary := stats.Compute(events, anomaly.Detect(events))
		if opts.json {
			return writeJSON(stdout, summary)
		}
		render.Stats(summary, stdout, theme)
		return nil
	case "anomalies":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		findings := anomaly.Detect(events)
		if opts.json {
			if findings == nil {
				findings = []anomaly.Anomaly{}
			}
			return writeJSON(stdout, findings)
		}
		render.Anomalies(findings, stdout, theme)
		return nil
	case "record":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		return render.Record(events, cmdArgs[0], cmdArgs[1], stdout, theme)
	case "replay":
		events, err := loadEvents(opts)
		if err != nil {
			return err
		}
		render.Replay(events, stdout, render.ReplayOptions{
			Speed:   opts.speed,
			Limit:   opts.limit,
			Animate: isTerminal(stdout),
			Theme:   theme,
		})
		return nil
	case "demo":
		return runDemo(opts, stdout, theme)
	case "version":
		fmt.Fprintln(stdout, version)
		return nil
	default:
		fmt.Fprintf(stderr, "cdcviz: unknown command %q\n\n", cmd)
		fmt.Fprint(stderr, usage)
		return errUsage
	}
}

func runDemo(opts options, out io.Writer, theme *render.Theme) error {
	events := demo.Events()
	if opts.json {
		enc := json.NewEncoder(out)
		for _, ev := range events {
			if err := enc.Encode(ev); err != nil {
				return err
			}
		}
		return nil
	}

	findings := anomaly.Detect(events)
	render.Timeline(events, out, render.TimelineOptions{
		Width: opts.width, MaxRows: opts.maxRows, Theme: theme,
	})
	fmt.Fprintln(out)
	render.Tables(stats.Compute(events, findings), out)
	fmt.Fprintln(out)
	render.Anomalies(findings, out, theme)
	return nil
}

func loadEvents(opts options) ([]event.Event, error) {
	var reader io.Reader = os.Stdin
	if opts.input != "" && opts.input != "-" {
		f, err := os.Open(opts.input)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		reader = f
	} else if isTerminal(os.Stdin) {
		return nil, fmt.Errorf("no input: pipe NDJSON, pass --input FILE, or run `cdcviz demo`")
	}

	parser := event.Parser{Format: opts.format, PKField: opts.pkField}
	events, err := event.NewDecoder(reader, parser).All()
	if err != nil {
		if opts.input != "" && opts.input != "-" {
			return nil, fmt.Errorf("%s: %w", opts.input, err)
		}
		return nil, err
	}
	return events, nil
}

func parseArgs(args []string) (options, []string, error) {
	opts := options{
		format:  event.FormatAuto,
		color:   "auto",
		width:   100,
		maxRows: 20,
		speed:   8,
	}
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			return opts, nil, nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			rest = append(rest, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		value := ""
		hasValue := false
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, value, hasValue = name[:eq], name[eq+1:], true
		}

		next := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag --%s needs a value", name)
			}
			i++
			return args[i], nil
		}

		var err error
		switch name {
		case "json":
			opts.json = true
			if hasValue {
				opts.json, err = strconv.ParseBool(value)
			}
		case "input", "i":
			opts.input, err = next()
		case "format":
			var raw string
			raw, err = next()
			if err == nil {
				opts.format, err = event.ParseFormat(raw)
			}
		case "pk-field":
			opts.pkField, err = next()
		case "color":
			opts.color, err = next()
			if err == nil && opts.color != "auto" && opts.color != "always" && opts.color != "never" {
				err = fmt.Errorf("--color must be auto, always, or never")
			}
		case "width":
			var raw string
			raw, err = next()
			if err == nil {
				opts.width, err = strconv.Atoi(raw)
				if err == nil && opts.width <= 0 {
					err = fmt.Errorf("--width must be positive")
				}
			}
		case "max-rows":
			var raw string
			raw, err = next()
			if err == nil {
				opts.maxRows, err = strconv.Atoi(raw)
				if err == nil && opts.maxRows <= 0 {
					err = fmt.Errorf("--max-rows must be positive")
				}
			}
		case "speed":
			var raw string
			raw, err = next()
			if err == nil {
				opts.speed, err = strconv.ParseFloat(raw, 64)
				if err == nil && (math.IsNaN(opts.speed) || math.IsInf(opts.speed, 0) || opts.speed < 0 ||
					(opts.speed > 0 && float64(time.Second)/opts.speed >= float64(math.MaxInt64))) {
					err = fmt.Errorf("--speed must be finite and nonnegative, with an interval below 292 years")
				}
			}
		case "limit":
			var raw string
			raw, err = next()
			if err == nil {
				opts.limit, err = strconv.Atoi(raw)
				if err == nil && opts.limit < 0 {
					err = fmt.Errorf("--limit must not be negative")
				}
			}
		default:
			err = fmt.Errorf("unknown flag --%s", name)
		}
		if err != nil {
			return opts, nil, err
		}
	}
	return opts, rest, nil
}

func writeJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func colorEnabled(mode string, out io.Writer) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		return isTerminal(out)
	}
}

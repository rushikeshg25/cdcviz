package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/event"
	"github.com/rushikeshg25/cdcviz/internal/stats"
)

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func inputFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// demoJSON returns the built-in scenario serialized as a native NDJSON stream.
func demoJSON(t *testing.T) string {
	t.Helper()
	out, _, err := runCLI(t, "demo", "--json")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestParseArgs(t *testing.T) {
	opts, rest, err := parseArgs([]string{
		"--width=80", "timeline", "--input", "x.jsonl", "--color", "always",
		"--format", "debezium", "--max-rows", "5", "--speed", "0", "--limit", "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.width != 80 || opts.input != "x.jsonl" || opts.color != "always" ||
		opts.format != event.FormatDebezium || opts.maxRows != 5 || opts.speed != 0 || opts.limit != 3 {
		t.Fatalf("opts = %+v", opts)
	}
	if strings.Join(rest, " ") != "timeline" {
		t.Fatalf("rest = %v", rest)
	}

	opts, _, err = parseArgs([]string{"anomalies", "--json", "--pk-field", "uid"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.json || opts.pkField != "uid" {
		t.Fatalf("opts = %+v", opts)
	}
}

func TestParseArgsRejectsBadValues(t *testing.T) {
	cases := [][]string{
		{"--width", "0"},
		{"--width", "abc"},
		{"--max-rows", "0"},
		{"--speed", "-1"},
		{"--limit", "-2"},
		{"--color", "sometimes"},
		{"--format", "xml"},
		{"--nope"},
		{"--input"},
	}
	for _, args := range cases {
		if _, _, err := parseArgs(args); err == nil {
			t.Fatalf("parseArgs(%v) accepted invalid input", args)
		}
	}
}

func TestDemoView(t *testing.T) {
	out, _, err := runCLI(t, "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Timeline", "17 events", "TABLE", "TOTAL", "Anomalies", "8 findings"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDemoJSONRoundTrip(t *testing.T) {
	raw := demoJSON(t)
	events, err := event.NewDecoder(strings.NewReader(raw), event.Parser{}).All()
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if len(events) != 17 {
		t.Fatalf("events = %d, want 17", len(events))
	}
}

func TestTimelineCommand(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "timeline", "--input", path, "--color", "never")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "17 events | 4 records") || !strings.Contains(out, "users/1") {
		t.Fatalf("unexpected timeline:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("--color never emitted escapes")
	}
}

func TestColorAlways(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "timeline", "--input", path, "--color", "always")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("--color always emitted no escapes")
	}
}

func TestStatsCommand(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "stats", "--input", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Summary", "17", "Operations", "Anomalies", "8"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatsJSON(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "stats", "--input", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var summary stats.Summary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if summary.Events != 17 || summary.Anomalies != 8 || summary.Ops["insert"] != 5 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestAnomaliesCommand(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "anomalies", "--input", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8 findings") || !strings.Contains(out, "out-of-order-lsn") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestAnomaliesJSON(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "anomalies", "--input", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var findings []anomaly.Anomaly
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(findings) != 8 {
		t.Fatalf("findings = %d, want 8", len(findings))
	}
}

func TestAnomaliesJSONEmptyIsArray(t *testing.T) {
	path := inputFile(t, `{"op":"i","table":"t","pk":"1","after":{"id":1}}`+"\n")
	out, _, err := runCLI(t, "anomalies", "--input", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("output = %q, want []", out)
	}
}

func TestRecordCommand(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "record", "users", "1", "--input", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "users/1") || !strings.Contains(out, "7 events") || !strings.Contains(out, "UPDATE") {
		t.Fatalf("unexpected record output:\n%s", out)
	}
}

func TestRecordCommandMissing(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	if _, _, err := runCLI(t, "record", "users", "404", "--input", path); err == nil {
		t.Fatal("expected not-found error")
	}
	if _, _, err := runCLI(t, "record", "users", "--input", path); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestReplayCommand(t *testing.T) {
	path := inputFile(t, demoJSON(t))
	out, _, err := runCLI(t, "replay", "--input", path, "--speed", "0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[17/17]") || !strings.Contains(out, "replayed 17/17 events") {
		t.Fatalf("unexpected replay output:\n%s", out)
	}
	if strings.Contains(out, "\r") {
		t.Fatal("non-terminal replay animated output")
	}
}

func TestLoadErrorMentionsFileAndLine(t *testing.T) {
	path := inputFile(t, "{\"op\":\"i\",\"table\":\"t\",\"pk\":\"1\",\"after\":{\"id\":1}}\nnot json\n")
	_, _, err := runCLI(t, "timeline", "--input", path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmptyInput(t *testing.T) {
	path := inputFile(t, "")
	out, _, err := runCLI(t, "timeline", "--input", path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "no events" {
		t.Fatalf("output = %q", out)
	}
}

func TestVersion(t *testing.T) {
	out, _, err := runCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != version {
		t.Fatalf("version = %q, want %q", out, version)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, err := runCLI(t, "bogus")
	if !errors.Is(err, errUsage) {
		t.Fatalf("err = %v, want errUsage", err)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestHelp(t *testing.T) {
	out, _, err := runCLI(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cdcviz", "timeline", "debezium", "--json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

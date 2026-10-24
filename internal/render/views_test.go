package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/event"
	"github.com/rushikeshg25/cdcviz/internal/stats"
)

func summaryFor(events []event.Event, findings []anomaly.Anomaly) stats.Summary {
	return stats.Compute(events, findings)
}

func TestTables(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "users", "1"),
		mkEvent(1, event.Update, "users", "1"),
		mkEvent(2, event.Insert, "orders", "9"),
	}
	var buf bytes.Buffer
	Tables(summaryFor(events, nil), &buf)
	out := buf.String()
	for _, want := range []string{"TABLE", "EVENTS", "users", "orders", "TOTAL", "2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestTablesEmpty(t *testing.T) {
	var buf bytes.Buffer
	Tables(stats.Compute(nil, nil), &buf)
	if strings.TrimSpace(buf.String()) != "no events" {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRecord(t *testing.T) {
	events := []event.Event{
		{TS: base, Op: event.Insert, Table: "users", PK: "1", After: event.Record{"id": 1, "name": "Ada"}},
		{TS: base.Add(time.Second), Op: event.Update, Table: "users", PK: "2", Before: event.Record{"id": 2}, After: event.Record{"id": 2, "name": "Grace"}},
		{TS: base.Add(2 * time.Second), Op: event.Update, Table: "users", PK: "1", Before: event.Record{"id": 1, "name": "Ada"}, After: event.Record{"id": 1, "name": "Ada L"}},
		{TS: base.Add(3 * time.Second), Op: event.Delete, Table: "users", PK: "1", Before: event.Record{"id": 1, "name": "Ada L"}},
	}
	var buf bytes.Buffer
	if err := Record(events, "users", "1", &buf, NewTheme(false)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"users/1", "3 events", "#1", "INSERT", `name: "Ada"`,
		"#2", "UPDATE", `name: "Ada" -> "Ada L"`, "#3", "DELETE",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Grace") {
		t.Fatalf("included another record's history:\n%s", out)
	}
}

func TestRecordNotFound(t *testing.T) {
	var buf bytes.Buffer
	err := Record(nil, "users", "404", &buf, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnomalies(t *testing.T) {
	findings := []anomaly.Anomaly{
		{Line: 2, Table: "users", PK: "1", Kind: anomaly.UpdateBeforeInsert, Severity: anomaly.Error, Message: "update for a key that was never inserted"},
		{Line: 5, Table: "orders", PK: "9", Kind: anomaly.NoOpUpdate, Severity: anomaly.Info, LSN: 7},
	}
	var buf bytes.Buffer
	Anomalies(findings, &buf, NewTheme(false))
	out := buf.String()
	for _, want := range []string{
		"2 findings", "1 error", "0 warning", "1 info", "line 2", "users/1",
		"update-before-insert", "update for a key", "line 5", "no-op-update", "lsn=7",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestAnomaliesClean(t *testing.T) {
	var buf bytes.Buffer
	Anomalies(nil, &buf, NewTheme(false))
	if !strings.Contains(buf.String(), "no anomalies detected") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestStats(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "users", "1"),
		mkEvent(2, event.Update, "users", "1"),
	}
	findings := []anomaly.Anomaly{
		{Table: "users", PK: "1", Kind: anomaly.DuplicateEvent, Severity: anomaly.Warning},
	}
	var buf bytes.Buffer
	Stats(summaryFor(events, findings), &buf, NewTheme(false))
	out := buf.String()
	for _, want := range []string{
		"Summary", "events", "2", "records", "1", "tables", "1",
		"throughput", "1.00 events/sec", "Operations", "insert", "1",
		"update", "Anomalies", "warning", "duplicate-event",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatsEmpty(t *testing.T) {
	var buf bytes.Buffer
	Stats(stats.Compute(nil, nil), &buf, NewTheme(false))
	if strings.TrimSpace(buf.String()) != "no events" {
		t.Fatalf("output = %q", buf.String())
	}
}

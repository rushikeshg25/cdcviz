package demo

import (
	"reflect"
	"testing"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/stats"
)

func TestScenarioIsWellFormed(t *testing.T) {
	events := Events()
	if len(events) != 17 {
		t.Fatalf("events = %d, want 17", len(events))
	}
	for i, ev := range events {
		if err := ev.Validate(); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if ev.TS.IsZero() {
			t.Fatalf("event %d has no timestamp", i)
		}
		if ev.Line != i+1 {
			t.Fatalf("event %d line = %d", i, ev.Line)
		}
	}
}

func TestScenarioCoversAnomalyKinds(t *testing.T) {
	findings := anomaly.Detect(Events())
	if len(findings) != 8 {
		t.Fatalf("findings = %d, want 8: %v", len(findings), findings)
	}
	got := map[anomaly.Kind]bool{}
	for _, finding := range findings {
		got[finding.Kind] = true
	}
	want := []anomaly.Kind{
		anomaly.DuplicateEvent,
		anomaly.DeleteBeforeInsert,
		anomaly.UpdateAfterDelete,
		anomaly.Resurrection,
		anomaly.DuplicateInsert,
		anomaly.OutOfOrderLSN,
		anomaly.StaleTimestamp,
		anomaly.NoOpUpdate,
	}
	for _, kind := range want {
		if !got[kind] {
			t.Fatalf("missing anomaly kind %s in %v", kind, findings)
		}
	}
}

func TestScenarioStats(t *testing.T) {
	events := Events()
	findings := anomaly.Detect(events)
	summary := stats.Compute(events, findings)

	if summary.Events != 17 || summary.Keys != 4 || len(summary.Tables) != 2 {
		t.Fatalf("events/keys/tables = %d/%d/%d", summary.Events, summary.Keys, len(summary.Tables))
	}
	if summary.Ops["insert"] != 5 || summary.Ops["update"] != 9 || summary.Ops["delete"] != 2 || summary.Ops["read"] != 1 {
		t.Fatalf("ops = %+v", summary.Ops)
	}
	if summary.Tables["users"].Events != 11 || summary.Tables["orders"].Events != 6 {
		t.Fatalf("tables = users:%+v orders:%+v", summary.Tables["users"], summary.Tables["orders"])
	}
	if summary.Anomalies != 8 {
		t.Fatalf("anomalies = %d, want 8", summary.Anomalies)
	}
	if summary.DurationSeconds != 23 {
		t.Fatalf("duration = %v, want 23", summary.DurationSeconds)
	}
}

func TestScenarioIsDeterministic(t *testing.T) {
	if !reflect.DeepEqual(Events(), Events()) {
		t.Fatal("scenario differs between calls")
	}
}

package stats

import (
	"testing"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/event"
)

var base = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func mk(seconds int, op event.Op, table, pk string) event.Event {
	return event.Event{
		TS:    base.Add(time.Duration(seconds) * time.Second),
		Op:    op,
		Table: table,
		PK:    pk,
	}
}

func TestCompute(t *testing.T) {
	events := []event.Event{
		mk(0, event.Insert, "users", "1"),
		mk(1, event.Update, "users", "1"),
		mk(2, event.Update, "users", "2"),
		mk(4, event.Insert, "orders", "10"),
		mk(6, event.Delete, "users", "2"),
		mk(8, event.Read, "orders", "10"),
	}
	anomalies := []anomaly.Anomaly{
		{Table: "users", Kind: anomaly.OutOfOrderLSN, Severity: anomaly.Warning},
		{Table: "users", Kind: anomaly.DuplicateEvent, Severity: anomaly.Warning},
		{Table: "orders", Kind: anomaly.NoOpUpdate, Severity: anomaly.Info},
	}

	s := Compute(events, anomalies)
	if s.Events != 6 || s.Keys != 3 {
		t.Fatalf("events/keys = %d/%d, want 6/3", s.Events, s.Keys)
	}
	if s.Ops["insert"] != 2 || s.Ops["update"] != 2 || s.Ops["delete"] != 1 || s.Ops["read"] != 1 {
		t.Fatalf("ops = %+v", s.Ops)
	}
	if !s.FirstTS.Equal(base) || !s.LastTS.Equal(base.Add(8*time.Second)) {
		t.Fatalf("time range = %v -> %v", s.FirstTS, s.LastTS)
	}
	if s.DurationSeconds != 8 {
		t.Fatalf("duration = %v, want 8", s.DurationSeconds)
	}
	if s.EventsPerSecond != 0.75 {
		t.Fatalf("events/sec = %v, want 0.75", s.EventsPerSecond)
	}

	users := s.Tables["users"]
	if users.Events != 4 || users.Inserts != 1 || users.Updates != 2 || users.Deletes != 1 || users.Keys != 2 || users.Anomalies != 2 {
		t.Fatalf("users = %+v", users)
	}
	if orders := s.Tables["orders"]; orders.Events != 2 || orders.Reads != 1 || orders.Keys != 1 || orders.Anomalies != 1 {
		t.Fatalf("orders = %+v", orders)
	}
	if s.Anomalies != 3 || s.AnomaliesBySeverity["warning"] != 2 || s.AnomaliesByKind["no-op-update"] != 1 {
		t.Fatalf("anomaly summary = %+v / %+v", s.AnomaliesByKind, s.AnomaliesBySeverity)
	}

	names := s.TableNames()
	if len(names) != 2 || names[0] != "orders" || names[1] != "users" {
		t.Fatalf("table names = %v", names)
	}
	counts := s.KindCounts()
	if len(counts) != 3 {
		t.Fatalf("kind counts = %+v", counts)
	}
	for i, count := range counts {
		if count.Count != 1 {
			t.Fatalf("kind count %d = %+v, want 1", i, count)
		}
		if i > 0 && counts[i-1].Kind > count.Kind {
			t.Fatalf("kind counts not sorted deterministically: %+v", counts)
		}
	}
}

func TestComputeEmpty(t *testing.T) {
	s := Compute(nil, nil)
	if s.Events != 0 || s.Keys != 0 || s.Anomalies != 0 {
		t.Fatalf("summary = %+v", s)
	}
	if s.DurationSeconds != 0 || s.EventsPerSecond != 0 {
		t.Fatalf("duration/throughput = %v/%v", s.DurationSeconds, s.EventsPerSecond)
	}
	if s.TableNames() == nil || len(s.TableNames()) != 0 {
		t.Fatalf("table names = %v", s.TableNames())
	}
}

func TestComputeIgnoresZeroTimestamps(t *testing.T) {
	events := []event.Event{
		{Op: event.Insert, Table: "t", PK: "1"},
		{Op: event.Insert, Table: "t", PK: "2", TS: base},
	}
	s := Compute(events, nil)
	if !s.FirstTS.Equal(base) || !s.LastTS.Equal(base) {
		t.Fatalf("range = %v -> %v", s.FirstTS, s.LastTS)
	}
	if s.DurationSeconds != 0 || s.EventsPerSecond != 0 {
		t.Fatalf("duration = %v, throughput = %v", s.DurationSeconds, s.EventsPerSecond)
	}
}

func TestAnomalyForUnknownTableIsCountedButNotAttributed(t *testing.T) {
	s := Compute([]event.Event{{Op: event.Insert, Table: "t", PK: "1"}}, []anomaly.Anomaly{
		{Table: "ghost", Kind: anomaly.DuplicateEvent, Severity: anomaly.Warning},
	})
	if s.Anomalies != 1 {
		t.Fatalf("anomalies = %d, want 1", s.Anomalies)
	}
	if s.Tables["t"].Anomalies != 0 {
		t.Fatalf("unexpected table anomaly count: %+v", s.Tables["t"])
	}
}

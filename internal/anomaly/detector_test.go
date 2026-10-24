package anomaly

import (
	"testing"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

var base = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func mk(line int, lsn uint64, seconds int, op event.Op, pk string, before, after event.Record) event.Event {
	return event.Event{
		Line:   line,
		LSN:    lsn,
		TS:     base.Add(time.Duration(seconds) * time.Second),
		Op:     op,
		Table:  "users",
		PK:     pk,
		Before: before,
		After:  after,
	}
}

func kinds(as []Anomaly) map[Kind]int {
	out := map[Kind]int{}
	for _, a := range as {
		out[a.Kind]++
	}
	return out
}

func TestCleanStreamHasNoAnomalies(t *testing.T) {
	events := []event.Event{
		mk(1, 1, 0, event.Insert, "1", nil, event.Record{"id": 1, "name": "Ada"}),
		mk(2, 2, 1, event.Update, "1", event.Record{"id": 1, "name": "Ada"}, event.Record{"id": 1, "name": "Ada L"}),
		mk(3, 3, 2, event.Insert, "2", nil, event.Record{"id": 2, "name": "Grace"}),
		mk(4, 4, 3, event.Delete, "1", event.Record{"id": 1, "name": "Ada L"}, nil),
	}
	if got := Detect(events); len(got) != 0 {
		t.Fatalf("unexpected anomalies: %v", got)
	}
}

func TestUpdateBeforeInsert(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 0, event.Update, "1", event.Record{"id": 1}, event.Record{"id": 1, "v": 2}),
	})
	if len(as) != 1 || as[0].Kind != UpdateBeforeInsert || as[0].Severity != Error {
		t.Fatalf("anomalies = %v", as)
	}
	if as[0].Line != 1 || as[0].Key() != "users/1" {
		t.Fatalf("metadata = %+v", as[0])
	}
}

func TestDeleteBeforeInsert(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 0, event.Delete, "9", event.Record{"id": 9}, nil),
	})
	if len(as) != 1 || as[0].Kind != DeleteBeforeInsert {
		t.Fatalf("anomalies = %v", as)
	}
}

func TestDuplicateInsert(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 0, event.Insert, "1", nil, event.Record{"id": 1, "v": 1}),
		mk(2, 2, 1, event.Insert, "1", nil, event.Record{"id": 1, "v": 2}),
	})
	if got := kinds(as); got[DuplicateInsert] != 1 || len(as) != 1 {
		t.Fatalf("anomalies = %v", as)
	}
}

func TestUpdateAndDeleteAfterDelete(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 0, event.Insert, "1", nil, event.Record{"id": 1}),
		mk(2, 2, 1, event.Delete, "1", event.Record{"id": 1}, nil),
		mk(3, 3, 2, event.Update, "1", event.Record{"id": 1}, event.Record{"id": 1, "v": 2}),
		mk(4, 4, 3, event.Delete, "1", event.Record{"id": 1}, nil),
		mk(5, 5, 4, event.Insert, "1", nil, event.Record{"id": 1, "v": 3}),
	})
	got := kinds(as)
	if got[UpdateAfterDelete] != 1 || got[DeleteAfterDelete] != 1 || got[Resurrection] != 1 {
		t.Fatalf("anomalies = %v", as)
	}
}

func TestNoOpUpdate(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 0, event.Insert, "1", nil, event.Record{"id": 1, "v": 1}),
		mk(2, 2, 1, event.Update, "1", event.Record{"id": 1, "v": 1}, event.Record{"id": 1, "v": 1}),
	})
	if len(as) != 1 || as[0].Kind != NoOpUpdate || as[0].Severity != Info {
		t.Fatalf("anomalies = %v", as)
	}
}

func TestDuplicateEvent(t *testing.T) {
	first := mk(1, 1, 0, event.Insert, "1", nil, event.Record{"id": 1})
	duplicate := first
	duplicate.Line = 2
	as := Detect([]event.Event{first, duplicate})
	if len(as) != 1 || as[0].Kind != DuplicateEvent {
		t.Fatalf("anomalies = %v", as)
	}
	if as[0].Message != "duplicate delivery of the event on line 1" {
		t.Fatalf("message = %q", as[0].Message)
	}
}

func TestOutOfOrderLSN(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 10, 0, event.Insert, "1", nil, event.Record{"id": 1, "v": 1}),
		mk(2, 5, 1, event.Update, "1", event.Record{"id": 1, "v": 1}, event.Record{"id": 1, "v": 2}),
		mk(3, 12, 2, event.Update, "1", event.Record{"id": 1, "v": 2}, event.Record{"id": 1, "v": 3}),
	})
	if len(as) != 1 || as[0].Kind != OutOfOrderLSN {
		t.Fatalf("anomalies = %v", as)
	}
	if as[0].Message != "lsn moved backwards: 10 -> 5" {
		t.Fatalf("message = %q", as[0].Message)
	}
}

func TestStaleTimestamp(t *testing.T) {
	as := Detect([]event.Event{
		mk(1, 1, 10, event.Insert, "1", nil, event.Record{"id": 1, "v": 1}),
		mk(2, 2, 5, event.Update, "1", event.Record{"id": 1, "v": 1}, event.Record{"id": 1, "v": 2}),
	})
	if len(as) != 1 || as[0].Kind != StaleTimestamp {
		t.Fatalf("anomalies = %v", as)
	}
}

func TestMissingTimestampsAreIgnored(t *testing.T) {
	first := mk(1, 0, 0, event.Insert, "1", nil, event.Record{"id": 1, "v": 1})
	first.TS = time.Time{}
	second := mk(2, 0, 0, event.Update, "1", event.Record{"id": 1, "v": 1}, event.Record{"id": 1, "v": 2})
	second.TS = time.Time{}
	if as := Detect([]event.Event{first, second}); len(as) != 0 {
		t.Fatalf("anomalies = %v", as)
	}
}

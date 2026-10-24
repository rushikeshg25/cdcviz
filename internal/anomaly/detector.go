// Package anomaly detects stream integrity problems in a CDC event stream:
// missing rows, duplicated deliveries, resurrected keys, and events that move
// backwards in log or wall-clock order.
package anomaly

import (
	"fmt"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

// Kind classifies a detected problem.
type Kind string

const (
	OutOfOrderLSN      Kind = "out-of-order-lsn"
	StaleTimestamp     Kind = "stale-timestamp"
	UpdateBeforeInsert Kind = "update-before-insert"
	DeleteBeforeInsert Kind = "delete-before-insert"
	DuplicateInsert    Kind = "duplicate-insert"
	UpdateAfterDelete  Kind = "update-after-delete"
	DeleteAfterDelete  Kind = "delete-after-delete"
	Resurrection       Kind = "resurrection"
	NoOpUpdate         Kind = "no-op-update"
	DuplicateEvent     Kind = "duplicate-event"
)

// Severity ranks how much attention an anomaly deserves.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Anomaly is one finding tied to a specific input event.
type Anomaly struct {
	Index    int       `json:"index"`
	Line     int       `json:"line"`
	Kind     Kind      `json:"kind"`
	Severity Severity  `json:"severity"`
	Table    string    `json:"table"`
	PK       string    `json:"pk"`
	LSN      uint64    `json:"lsn,omitempty"`
	TS       time.Time `json:"ts"`
	Message  string    `json:"message"`
}

// Key identifies the record the finding belongs to.
func (a Anomaly) Key() string { return a.Table + "/" + a.PK }

func (a Anomaly) String() string {
	return fmt.Sprintf("[%s] %s %s (line %d): %s", a.Severity, a.Kind, a.Key(), a.Line, a.Message)
}

type keyState struct {
	seen    bool
	deleted bool
	lastTS  time.Time
}

// Detect walks events in input order and reports integrity anomalies. Detection
// is stateful per table/primary key and per stream.
func Detect(events []event.Event) []Anomaly {
	var out []Anomaly

	states := make(map[event.RecordID]*keyState)
	signatures := make(map[eventSignature]int)
	var lastLSN uint64

	add := func(index int, ev event.Event, kind Kind, severity Severity, format string, args ...any) {
		out = append(out, Anomaly{
			Index:    index,
			Line:     ev.Line,
			Kind:     kind,
			Severity: severity,
			Table:    ev.Table,
			PK:       ev.PK,
			LSN:      ev.LSN,
			TS:       ev.TS,
			Message:  fmt.Sprintf(format, args...),
		})
	}

	for i, ev := range events {
		key := ev.Identity()
		state := states[key]
		if state == nil {
			state = &keyState{}
			states[key] = state
		}

		if ev.LSN > 0 && lastLSN > 0 && ev.LSN < lastLSN {
			add(i, ev, OutOfOrderLSN, Warning, "lsn moved backwards: %d -> %d", lastLSN, ev.LSN)
		}
		if ev.LSN > lastLSN {
			lastLSN = ev.LSN
		}

		if !ev.TS.IsZero() && !state.lastTS.IsZero() && ev.TS.Before(state.lastTS) {
			add(i, ev, StaleTimestamp, Warning,
				"event time %s is older than the previous event for this key (%s)",
				event.FormatTime(ev.TS), event.FormatTime(state.lastTS))
		}

		sig := signature(ev)
		duplicate := false
		if previousLine, ok := signatures[sig]; ok {
			add(i, ev, DuplicateEvent, Warning, "duplicate delivery of the event on line %d", previousLine)
			duplicate = true
		} else {
			signatures[sig] = ev.Line
		}

		// A duplicate has already been folded into the per-key state; running
		// the state transition again would report the same root cause twice.
		if duplicate {
			continue
		}

		switch ev.Op {
		case event.Insert, event.Read:
			if ev.Op == event.Insert {
				switch {
				case state.deleted:
					add(i, ev, Resurrection, Warning, "insert for a key that was previously deleted")
				case state.seen:
					add(i, ev, DuplicateInsert, Warning, "insert for a key that is already live")
				}
			}
			state.seen = true
			state.deleted = false
		case event.Update:
			switch {
			case !state.seen:
				add(i, ev, UpdateBeforeInsert, Error, "update for a key that was never inserted")
			case state.deleted:
				add(i, ev, UpdateAfterDelete, Warning, "update for a key after it was deleted")
			}
			if len(ev.Changes()) == 0 {
				add(i, ev, NoOpUpdate, Info, "update has no field changes")
			}
			state.seen = true
		case event.Delete:
			switch {
			case !state.seen:
				add(i, ev, DeleteBeforeInsert, Error, "delete for a key that was never inserted")
			case state.deleted:
				add(i, ev, DeleteAfterDelete, Warning, "delete for a key that is already deleted")
			}
			state.seen = true
			state.deleted = true
		}

		if ev.TS.After(state.lastTS) {
			state.lastTS = ev.TS
		}
	}

	return out
}

// Keep fields separate: concatenating table and key with a delimiter can make
// different events look like duplicate deliveries.
type eventSignature struct {
	lsn           uint64
	ts            string
	op            event.Op
	record        event.RecordID
	before, after string
}

func signature(ev event.Event) eventSignature {
	return eventSignature{
		lsn: ev.LSN, ts: ev.TS.UTC().Format(time.RFC3339Nano), op: ev.Op,
		record: ev.Identity(), before: event.CanonicalRecord(ev.Before), after: event.CanonicalRecord(ev.After),
	}
}

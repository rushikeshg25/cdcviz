// Package event defines the normalized change event that cdcviz consumes,
// independent of the wire format it arrived in.
package event

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Op is a normalized change operation.
type Op string

const (
	Insert Op = "insert"
	Update Op = "update"
	Delete Op = "delete"
	Read   Op = "read"
)

// ParseOp maps Debezium single-letter ops and long names onto Op.
func ParseOp(raw string) (Op, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "c", "i", "create", "insert":
		return Insert, nil
	case "u", "update":
		return Update, nil
	case "d", "delete":
		return Delete, nil
	case "r", "read", "snapshot":
		return Read, nil
	default:
		return "", fmt.Errorf("unknown operation %q", raw)
	}
}

func (o Op) String() string { return string(o) }

// Record is one row image (the value of a before/after payload).
type Record map[string]any

// Keys returns the sorted field names of a record.
func (r Record) Keys() []string {
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Event is one change captured from a CDC stream.
type Event struct {
	LSN    uint64    `json:"lsn"`
	TS     time.Time `json:"ts"`
	Op     Op        `json:"op"`
	Table  string    `json:"table"`
	PK     string    `json:"pk"`
	Before Record    `json:"before,omitempty"`
	After  Record    `json:"after,omitempty"`
	// Line is the 1-based line in the input stream. It is not part of the
	// wire format; the decoder fills it in so diagnostics can point at it.
	Line int `json:"-"`
}

// Key identifies the record the event belongs to.
func (e Event) Key() string { return e.Table + "/" + e.PK }

// Validate checks the invariants required to render and analyze an event.
func (e Event) Validate() error {
	if e.Table == "" {
		return fmt.Errorf("event has no table")
	}
	if e.PK == "" {
		return fmt.Errorf("event has no primary key")
	}
	switch e.Op {
	case Insert, Read:
		if e.After == nil {
			return fmt.Errorf("%s event for %s has no after image", e.Op, e.Key())
		}
	case Update:
		if e.After == nil {
			return fmt.Errorf("update event for %s has no after image", e.Key())
		}
	case Delete:
		if e.Before == nil {
			return fmt.Errorf("delete event for %s has no before image", e.Key())
		}
	default:
		return fmt.Errorf("event for %s has unknown operation %q", e.Key(), e.Op)
	}
	return nil
}

// ChangeKeys returns the sorted union of keys in the before and after images.
func (e Event) ChangeKeys() []string {
	set := map[string]bool{}
	for key := range e.Before {
		set[key] = true
	}
	for key := range e.After {
		set[key] = true
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Changes describes what an update changed. Non-update events describe the
// whole relevant row image.
func (e Event) Changes() []string {
	switch e.Op {
	case Delete:
		return recordPairs(e.Before)
	case Update:
		var out []string
		for _, key := range e.ChangeKeys() {
			before, hadBefore := e.Before[key]
			after, hasAfter := e.After[key]
			switch {
			case !hadBefore && hasAfter:
				out = append(out, fmt.Sprintf("%s: (added) %s", key, FormatValue(after)))
			case hadBefore && !hasAfter:
				out = append(out, fmt.Sprintf("%s: (removed) %s", key, FormatValue(before)))
			case !equalValues(before, after):
				out = append(out, fmt.Sprintf("%s: %s -> %s", key, FormatValue(before), FormatValue(after)))
			}
		}
		return out
	default:
		return recordPairs(e.After)
	}
}

// Describe renders the change payload as a one-line summary.
func (e Event) Describe() string {
	changes := e.Changes()
	if len(changes) == 0 {
		return "(no field changes)"
	}
	return strings.Join(changes, ", ")
}

func recordPairs(rec Record) []string {
	out := make([]string, 0, len(rec))
	for _, key := range rec.Keys() {
		out = append(out, fmt.Sprintf("%s: %s", key, FormatValue(rec[key])))
	}
	return out
}

func equalValues(a, b any) bool {
	aj, aerr := json.Marshal(a)
	bj, berr := json.Marshal(b)
	if aerr != nil || berr != nil {
		return false
	}
	return string(aj) == string(bj)
}

// FormatValue renders a JSON value for human display.
func FormatValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("%q", value)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

// FormatTime renders an event timestamp, or "--" when it is absent.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	return t.UTC().Format("15:04:05.000")
}

// CanonicalRecord serializes a record deterministically for deduplication.
func CanonicalRecord(rec Record) string {
	if rec == nil {
		return "null"
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Sprintf("%v", rec)
	}
	return string(raw)
}

// ScalarString renders an ID-like scalar without JSON quoting.
func ScalarString(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(raw)
	}
}

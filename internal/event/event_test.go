package event

import (
	"testing"
	"time"
)

func TestParseOpAliases(t *testing.T) {
	cases := map[string]Op{
		"i": Insert, "insert": Insert, "c": Insert, "CREATE": Insert,
		"u": Update, "update": Update,
		"d": Delete, "delete": Delete,
		"r": Read, "snapshot": Read,
	}
	for raw, want := range cases {
		got, err := ParseOp(raw)
		if err != nil {
			t.Fatalf("ParseOp(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseOp(%q) = %q, want %q", raw, got, want)
		}
	}
	if _, err := ParseOp("x"); err == nil {
		t.Fatal("expected error for unknown op")
	}
}

func TestValidatePayloadRequirements(t *testing.T) {
	base := Event{Table: "users", PK: "1", After: Record{"id": 1}}
	if err := (Event{Table: "users", PK: "1", Op: Insert, After: Record{"id": 1}}).Validate(); err != nil {
		t.Fatalf("valid insert rejected: %v", err)
	}
	if err := (Event{Table: "users", PK: "1", Op: Update, After: Record{"id": 1}}).Validate(); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
	if err := (Event{Table: "users", PK: "1", Op: Delete, Before: Record{"id": 1}}).Validate(); err != nil {
		t.Fatalf("valid delete rejected: %v", err)
	}

	cases := []struct {
		name string
		ev   Event
	}{
		{"no table", Event{PK: "1", Op: Insert, After: base.After}},
		{"no pk", Event{Table: "users", Op: Insert, After: base.After}},
		{"insert without after", Event{Table: "users", PK: "1", Op: Insert}},
		{"update without after", Event{Table: "users", PK: "1", Op: Update}},
		{"delete without before", Event{Table: "users", PK: "1", Op: Delete}},
		{"bad op", Event{Table: "users", PK: "1", Op: "merge", After: base.After}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.ev.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestChanges(t *testing.T) {
	ev := Event{
		Op:     Update,
		Before: Record{"name": "ada", "email": "ada@example.com", "legacy": "x"},
		After:  Record{"name": "Ada", "email": "ada@example.com", "country": "UK"},
	}
	got := ev.Changes()
	want := []string{
		`country: (added) "UK"`,
		`legacy: (removed) "x"`,
		`name: "ada" -> "Ada"`,
	}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("change %d = %q, want %q", i, got[i], want[i])
		}
	}

	if ev := (Event{Op: Insert, After: Record{"b": 2, "a": 1}}); ev.Describe() != "a: 1, b: 2" {
		t.Fatalf("describe = %q", ev.Describe())
	}
	if ev := (Event{Op: Update, Before: Record{"a": 1}, After: Record{"a": 1}}); ev.Describe() != "(no field changes)" {
		t.Fatalf("describe = %q", ev.Describe())
	}
}

func TestFormatValueAndScalar(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"nil":     {nil, "null"},
		"string":  {"hello", `"hello"`},
		"integer": {float64(42), "42"},
		"float":   {1.5, "1.5"},
		"bool":    {true, "true"},
	}
	for name, tc := range cases {
		if got := FormatValue(tc.value); got != tc.want {
			t.Fatalf("%s: FormatValue = %q, want %q", name, got, tc.want)
		}
	}
	if got := ScalarString(float64(7)); got != "7" {
		t.Fatalf("ScalarString = %q, want 7", got)
	}
	if got := ScalarString("abc"); got != "abc" {
		t.Fatalf("ScalarString = %q, want abc", got)
	}
}

func TestCanonicalRecordIsStable(t *testing.T) {
	a := Record{"b": 2, "a": 1}
	b := Record{"a": 1, "b": 2}
	if CanonicalRecord(a) != CanonicalRecord(b) {
		t.Fatal("canonical record depends on insertion order")
	}
	if CanonicalRecord(nil) != "null" {
		t.Fatal("nil record should encode as null")
	}
}

func TestFormatTime(t *testing.T) {
	if got := FormatTime(time.Time{}); got != "--" {
		t.Fatalf("zero time = %q", got)
	}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 6000000, time.UTC)
	if got := FormatTime(ts); got != "03:04:05.006" {
		t.Fatalf("FormatTime = %q", got)
	}
}

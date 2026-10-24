package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

var base = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func mkEvent(seconds int, op event.Op, table, pk string) event.Event {
	return event.Event{
		TS:    base.Add(time.Duration(seconds) * time.Second),
		Op:    op,
		Table: table,
		PK:    pk,
		Line:  seconds + 1,
	}
}

func TestTimelineEmpty(t *testing.T) {
	var buf bytes.Buffer
	Timeline(nil, &buf, TimelineOptions{})
	if strings.TrimSpace(buf.String()) != "no events" {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestTimelineLayout(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "users", "1"),
		mkEvent(1, event.Update, "users", "1"),
		mkEvent(2, event.Insert, "users", "2"),
		mkEvent(3, event.Update, "users", "1"),
		mkEvent(4, event.Delete, "users", "2"),
	}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 80, MaxRows: 10})

	out := buf.String()
	for _, want := range []string{
		"Timeline",
		"5 events | 2 records",
		"users/1",
		"users/2",
		"3 events",
		"2 events",
		"Legend:",
		"insert",
		"update",
		"delete",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}

	// Busiest record is rendered first.
	if strings.Index(out, "users/1") > strings.Index(out, "users/2") {
		t.Fatalf("rows are not sorted busiest first:\n%s", out)
	}

	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "users/") && len([]rune(line)) != 80 {
			t.Fatalf("row is %d columns, want 80: %q", len([]rune(line)), line)
		}
	}
}

func TestTimelineTruncatesLongKeys(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "very_long_table_name_that_will_not_fit", "1234567890"),
	}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 60})
	out := buf.String()
	if strings.Contains(out, "very_long_table_name_that_will_not_fit/1234567890") {
		t.Fatalf("key was not truncated:\n%s", out)
	}
	var label string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "very_long") {
			label = strings.SplitN(line, " ", 2)[0]
		}
	}
	if label == "" {
		t.Fatalf("truncated row missing:\n%s", out)
	}
	if len([]rune(label)) != 30 || !strings.HasSuffix(label, "~") {
		t.Fatalf("label = %q, want 30 chars ending in ~", label)
	}
}

func TestTimelineFitsNarrowWidthWithLongKey(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "very_long_table_name_that_will_not_fit", "1234567890"),
	}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 40})
	out := buf.String()
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "very_long") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("row missing:\n%s", out)
	}
	if got := len([]rune(row)); got != 40 {
		t.Fatalf("row is %d columns, want 40: %q", got, row)
	}
	label := strings.SplitN(row, " ", 2)[0]
	if !strings.HasSuffix(label, "~") {
		t.Fatalf("label = %q, want a truncation marker", label)
	}
}

func TestTimelineShowsDatesAcrossMidnight(t *testing.T) {
	first := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	events := []event.Event{
		{TS: first, Op: event.Insert, Table: "t", PK: "1"},
		{TS: first.Add(2 * time.Second), Op: event.Update, Table: "t", PK: "1"},
	}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 80})
	if !strings.Contains(buf.String(), "2026-12-31 23:59:59 -> 2027-01-01 00:00:01") {
		t.Fatalf("missing dated range:\n%s", buf.String())
	}
}

func TestTimelineMaxRows(t *testing.T) {
	var events []event.Event
	for i := 0; i < 5; i++ {
		events = append(events, mkEvent(i, event.Insert, "t", string(rune('a'+i))))
	}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 60, MaxRows: 2})
	out := buf.String()
	if got := strings.Count(out, "t/"); got != 2 {
		t.Fatalf("rendered %d rows, want 2:\n%s", got, out)
	}
	if !strings.Contains(out, "3 more records not shown") {
		t.Fatalf("missing hidden-row note:\n%s", out)
	}
}

func TestTimelineColor(t *testing.T) {
	events := []event.Event{mkEvent(0, event.Insert, "t", "1")}

	var plain bytes.Buffer
	Timeline(events, &plain, TimelineOptions{Width: 60, Theme: NewTheme(false)})
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatal("plain theme emitted ANSI escapes")
	}

	var colored bytes.Buffer
	Timeline(events, &colored, TimelineOptions{Width: 60, Theme: NewTheme(true)})
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Fatal("colored theme emitted no ANSI escapes")
	}
}

func TestTimelineWithoutTimestamps(t *testing.T) {
	events := []event.Event{{Op: event.Insert, Table: "t", PK: "1"}}
	var buf bytes.Buffer
	Timeline(events, &buf, TimelineOptions{Width: 60})
	if !strings.Contains(buf.String(), "-- -> --") {
		t.Fatalf("unexpected range rendering:\n%s", buf.String())
	}
}

func TestOpSymbol(t *testing.T) {
	cases := map[event.Op]string{
		event.Insert: "+",
		event.Update: "~",
		event.Delete: "-",
		event.Read:   ".",
	}
	for op, want := range cases {
		if got := OpSymbol(op); got != want {
			t.Fatalf("OpSymbol(%s) = %q, want %q", op, got, want)
		}
	}
}

func TestTimelineLimitPreservesSequenceAxis(t *testing.T) {
	events := []event.Event{
		mkEvent(0, event.Insert, "t", "a"),
		mkEvent(1, event.Insert, "t", "b"),
		mkEvent(2, event.Insert, "t", "c"),
		mkEvent(3, event.Delete, "t", "a"),
	}
	var full, limited bytes.Buffer
	Timeline(events, &full, TimelineOptions{Width: 80, MaxRows: 3})
	Timeline(events, &limited, TimelineOptions{Width: 80, MaxRows: 1})
	for _, line := range strings.Split(full.String(), "\n") {
		if strings.HasPrefix(line, "t/a") && !strings.Contains(limited.String(), line) {
			t.Fatalf("limiting rows changed the visible sequence axis:\n%s", limited.String())
		}
	}
	if strings.Contains(limited.String(), "t/b") || !strings.Contains(limited.String(), "2 more records") {
		t.Fatalf("incorrect row limit:\n%s", limited.String())
	}
}

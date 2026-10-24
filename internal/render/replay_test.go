package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

func replayEvents() []event.Event {
	return []event.Event{
		{TS: base, Op: event.Insert, Table: "users", PK: "1", After: event.Record{"id": 1, "name": "Ada"}},
		{TS: base.Add(time.Second), Op: event.Update, Table: "users", PK: "1", Before: event.Record{"id": 1, "name": "Ada"}, After: event.Record{"id": 1, "name": "Ada L"}},
		{TS: base.Add(2 * time.Second), Op: event.Delete, Table: "users", PK: "1", Before: event.Record{"id": 1, "name": "Ada L"}},
	}
}

func TestReplayPlain(t *testing.T) {
	var buf bytes.Buffer
	Replay(replayEvents(), &buf, ReplayOptions{Theme: NewTheme(false)})
	out := buf.String()
	for _, want := range []string{
		"[1/3]", "INSERT", "users/1", `name: "Ada"`,
		"[2/3]", "UPDATE", `name: "Ada" -> "Ada L"`,
		"[3/3]", "DELETE",
		"replayed 3/3 events", "insert 1, update 1, delete 1, read 0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") || strings.Contains(out, "\x1b[") {
		t.Fatalf("plain replay emitted terminal control characters:\n%q", out)
	}
}

func TestReplayAnimated(t *testing.T) {
	var buf bytes.Buffer
	Replay(replayEvents(), &buf, ReplayOptions{Animate: true, Theme: NewTheme(true)})
	out := buf.String()
	if !strings.Contains(out, "\r\x1b[2K") {
		t.Fatalf("animated replay did not clear the line:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("colored replay had no escapes")
	}
}

func TestReplayLimit(t *testing.T) {
	var buf bytes.Buffer
	Replay(replayEvents(), &buf, ReplayOptions{Limit: 2})
	out := buf.String()
	if !strings.Contains(out, "replayed 2/3 events (limit 2)") {
		t.Fatalf("missing limit summary:\n%s", out)
	}
	if strings.Contains(out, "[3/3]") {
		t.Fatalf("rendered beyond limit:\n%s", out)
	}
}

func TestReplaySpeedSleepsBetweenEvents(t *testing.T) {
	var slept []time.Duration
	var buf bytes.Buffer
	Replay(replayEvents(), &buf, ReplayOptions{
		Speed: 10,
		Sleep: func(d time.Duration) { slept = append(slept, d) },
	})
	if len(slept) != 3 {
		t.Fatalf("slept %d times, want 3", len(slept))
	}
	for _, d := range slept {
		if d != 100*time.Millisecond {
			t.Fatalf("sleep = %v, want 100ms", d)
		}
	}
}

func TestReplayZeroSpeedDoesNotSleep(t *testing.T) {
	var buf bytes.Buffer
	Replay(replayEvents(), &buf, ReplayOptions{
		Speed: 0,
		Sleep: func(time.Duration) { t.Fatal("unexpected sleep") },
	})
}

func TestReplayEmpty(t *testing.T) {
	var buf bytes.Buffer
	Replay(nil, &buf, ReplayOptions{})
	if strings.TrimSpace(buf.String()) != "no events" {
		t.Fatalf("output = %q", buf.String())
	}
}

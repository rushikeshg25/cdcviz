package event

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, parser Parser, line string) Event {
	t.Helper()
	ev, err := parser.ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ev
}

func TestParseNativeInsert(t *testing.T) {
	ev := mustParse(t, Parser{}, `{"lsn":1,"ts":"2026-01-01T10:00:00Z","op":"i","table":"users","pk":"1","after":{"id":1,"name":"Ada"}}`)
	if ev.LSN != 1 || ev.Op != Insert || ev.Table != "users" || ev.PK != "1" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if !ev.TS.Equal(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("ts = %v", ev.TS)
	}
	if ev.After["name"] != "Ada" {
		t.Fatalf("after = %+v", ev.After)
	}
}

func TestParseNativePKAliasesAndNumbers(t *testing.T) {
	cases := map[string]string{
		"pk":      `{"op":"d","table":"t","pk":42,"before":{"id":42}}`,
		"key":     `{"op":"d","table":"t","key":"k1","before":{"id":42}}`,
		"id":      `{"op":"d","table":"t","id":"abc","before":{"x":1}}`,
		"derived": `{"op":"d","table":"t","before":{"id":42}}`,
	}
	want := map[string]string{"pk": "42", "key": "k1", "id": "abc", "derived": "42"}
	for name, line := range cases {
		ev := mustParse(t, Parser{}, line)
		if ev.PK != want[name] {
			t.Fatalf("%s: pk = %q, want %q", name, ev.PK, want[name])
		}
	}
}

func TestParseNativeCustomPKField(t *testing.T) {
	ev := mustParse(t, Parser{PKField: "uid"}, `{"op":"i","table":"t","after":{"uid":99,"name":"x"}}`)
	if ev.PK != "99" {
		t.Fatalf("pk = %q, want 99", ev.PK)
	}
}

func TestParseNativeTimestampForms(t *testing.T) {
	cases := map[string]time.Time{
		`"2026-01-01T10:00:00.123Z"`: time.Date(2026, 1, 1, 10, 0, 0, 123000000, time.UTC),
		`1767261600000`:              time.UnixMilli(1767261600000),
		`"1767261600000"`:            time.UnixMilli(1767261600000),
		`1767261600`:                 time.Unix(1767261600, 0),
	}
	for raw, want := range cases {
		line := `{"op":"i","table":"t","pk":"1","ts":` + raw + `,"after":{"id":1}}`
		ev := mustParse(t, Parser{}, line)
		if !ev.TS.Equal(want) {
			t.Fatalf("ts %s = %v, want %v", raw, ev.TS, want)
		}
	}
}

func TestParseNativeLSNForms(t *testing.T) {
	cases := map[string]uint64{
		`"12345"`:     12345,
		`"0/16B6C50"`: 0x16B6C50,
		`987`:         987,
	}
	for raw, want := range cases {
		line := `{"op":"i","table":"t","pk":"1","lsn":` + raw + `,"after":{"id":1}}`
		ev := mustParse(t, Parser{}, line)
		if ev.LSN != want {
			t.Fatalf("lsn %s = %d, want %d", raw, ev.LSN, want)
		}
	}
}

func TestParseNativeErrors(t *testing.T) {
	cases := map[string]string{
		"bad json":       `{"op":`,
		"bad op":         `{"op":"merge","table":"t","pk":"1","after":{"id":1}}`,
		"missing table":  `{"op":"i","pk":"1","after":{"id":1}}`,
		"missing pk":     `{"op":"i","table":"t","after":{"name":"x"}}`,
		"missing after":  `{"op":"i","table":"t","pk":"1"}`,
		"missing before": `{"op":"d","table":"t","pk":"1"}`,
		"bad lsn":        `{"op":"i","table":"t","pk":"1","lsn":"nope","after":{"id":1}}`,
		"bad ts":         `{"op":"i","table":"t","pk":"1","ts":"yesterday","after":{"id":1}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (Parser{}).ParseLine([]byte(line)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseDebeziumPostgres(t *testing.T) {
	line := `{"payload":{"before":null,"after":{"id":1,"name":"Ada"},"source":{"table":"users","lsn":123456789,"ts_ms":1767261600000},"op":"c"}}`
	ev := mustParse(t, Parser{Format: FormatDebezium}, line)
	if ev.Op != Insert || ev.Table != "users" || ev.PK != "1" || ev.LSN != 123456789 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if !ev.TS.Equal(time.UnixMilli(1767261600000)) {
		t.Fatalf("ts = %v", ev.TS)
	}
}

func TestParseDebeziumMySQLWithKey(t *testing.T) {
	line := `{"payload":{"before":{"id":7,"name":"a"},"after":{"id":7,"name":"b"},"source":{"table":"users","pos":55,"ts_ms":1767261600000},"op":"u"},"key":{"id":7}}`
	ev := mustParse(t, Parser{}, line)
	if ev.Op != Update || ev.PK != "7" || ev.LSN != 55 {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestParseDebeziumPKField(t *testing.T) {
	line := `{"payload":{"before":null,"after":{"user_id":9},"source":{"table":"users","lsn":1,"ts_ms":1767261600000},"op":"r"}}`
	ev := mustParse(t, Parser{PKField: "user_id"}, line)
	if ev.Op != Read || ev.PK != "9" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestParseDebeziumMissingPK(t *testing.T) {
	line := `{"payload":{"after":{"name":"anon"},"source":{"table":"users","lsn":1},"op":"c"}}`
	if _, err := (Parser{}).ParseLine([]byte(line)); err == nil {
		t.Fatal("expected missing pk error")
	}
}

func TestAutoDetectsFormat(t *testing.T) {
	native := `{"op":"i","table":"t","pk":"1","after":{"id":1}}`
	debezium := `{"payload":{"op":"c","after":{"id":2},"source":{"table":"t","lsn":3}}}`

	if ev := mustParse(t, Parser{Format: FormatAuto}, native); ev.PK != "1" {
		t.Fatalf("native pk = %q", ev.PK)
	}
	if ev := mustParse(t, Parser{Format: FormatAuto}, debezium); ev.PK != "2" {
		t.Fatalf("debezium pk = %q", ev.PK)
	}
}

func TestParseFormat(t *testing.T) {
	for _, raw := range []string{"", "auto", "native", "debezium", "DEBEZIUM"} {
		if _, err := ParseFormat(raw); err != nil {
			t.Fatalf("ParseFormat(%q): %v", raw, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecoder(t *testing.T) {
	input := strings.Join([]string{
		`{"op":"i","table":"t","pk":"1","after":{"id":1}}`,
		``,
		`{"op":"u","table":"t","pk":"1","before":{"id":1},"after":{"id":1,"v":2}}`,
	}, "\n")

	dec := NewDecoder(strings.NewReader(input), Parser{})
	var events []Event
	for {
		ev, err := dec.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Line != 1 || events[1].Line != 3 {
		t.Fatalf("line numbers = %d,%d", events[0].Line, events[1].Line)
	}
}

func TestDecoderReportsLineNumbers(t *testing.T) {
	input := "{\"op\":\"i\",\"table\":\"t\",\"pk\":\"1\",\"after\":{\"id\":1}}\nnot-json\n"
	dec := NewDecoder(strings.NewReader(input), Parser{})
	if _, err := dec.Next(); err != nil {
		t.Fatal(err)
	}
	_, err := dec.Next()
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want line 2", err)
	}
}

func TestDecoderAll(t *testing.T) {
	input := `{"op":"i","table":"t","pk":"1","after":{"id":1}}
{"op":"d","table":"t","pk":"1","before":{"id":1}}`
	events, err := NewDecoder(strings.NewReader(input), Parser{}).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
}

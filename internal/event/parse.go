package event

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Format names the supported CDC wire formats.
type Format string

const (
	FormatAuto     Format = "auto"
	FormatNative   Format = "native"
	FormatDebezium Format = "debezium"
)

// ParseFormat validates a --format value.
func ParseFormat(raw string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(raw))) {
	case "", FormatAuto:
		return FormatAuto, nil
	case FormatNative:
		return FormatNative, nil
	case FormatDebezium:
		return FormatDebezium, nil
	default:
		return "", fmt.Errorf("unknown format %q (want auto, native, or debezium)", raw)
	}
}

// Parser turns one JSON line into a normalized event.
type Parser struct {
	Format  Format
	PKField string
}

// ParseLine parses a single line, auto-detecting the format when requested.
func (p Parser) ParseLine(line []byte) (Event, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Event{}, fmt.Errorf("empty line")
	}
	format := p.Format
	if format == "" || format == FormatAuto {
		format = detectFormat(line)
	}
	switch format {
	case FormatNative:
		return p.parseNative(line)
	case FormatDebezium:
		return p.parseDebezium(line)
	default:
		return Event{}, fmt.Errorf("unsupported format %q", format)
	}
}

func detectFormat(line []byte) Format {
	var probe struct {
		Payload json.RawMessage `json:"payload"`
		Table   string          `json:"table"`
	}
	if err := json.Unmarshal(line, &probe); err == nil {
		if len(probe.Payload) > 0 && probe.Table == "" {
			return FormatDebezium
		}
	}
	return FormatNative
}

type nativeWire struct {
	LSN    flexUint64 `json:"lsn"`
	TS     flexTime   `json:"ts"`
	Op     string     `json:"op"`
	Table  string     `json:"table"`
	PK     any        `json:"pk"`
	Key    any        `json:"key"`
	ID     any        `json:"id"`
	Before Record     `json:"before"`
	After  Record     `json:"after"`
}

func (p Parser) parseNative(line []byte) (Event, error) {
	var wire nativeWire
	if err := decodeJSON(line, &wire); err != nil {
		return Event{}, fmt.Errorf("native event: %w", err)
	}
	op, err := ParseOp(wire.Op)
	if err != nil {
		return Event{}, err
	}
	ev := Event{
		LSN:    uint64(wire.LSN),
		TS:     wire.TS.Time,
		Op:     op,
		Table:  strings.TrimSpace(wire.Table),
		Before: wire.Before,
		After:  wire.After,
	}
	ev.PK = p.pkFor([]any{wire.PK, wire.Key, wire.ID}, ev.Before, ev.After)
	if err := ev.Validate(); err != nil {
		return Event{}, err
	}
	return ev, nil
}

type debeziumWire struct {
	Payload struct {
		Before Record   `json:"before"`
		After  Record   `json:"after"`
		Op     string   `json:"op"`
		TSMS   flexTime `json:"ts_ms"`
		Source struct {
			Table string     `json:"table"`
			LSN   flexUint64 `json:"lsn"`
			Pos   flexUint64 `json:"pos"`
			TSMS  flexTime   `json:"ts_ms"`
		} `json:"source"`
	} `json:"payload"`
	Key   Record `json:"key"`
	Table string `json:"table"`
}

func (p Parser) parseDebezium(line []byte) (Event, error) {
	var wire debeziumWire
	if err := decodeJSON(line, &wire); err != nil {
		return Event{}, fmt.Errorf("debezium event: %w", err)
	}
	op, err := ParseOp(wire.Payload.Op)
	if err != nil {
		return Event{}, err
	}
	lsn := uint64(wire.Payload.Source.LSN)
	if lsn == 0 {
		lsn = uint64(wire.Payload.Source.Pos)
	}
	ts := wire.Payload.Source.TSMS.Time
	if ts.IsZero() {
		ts = wire.Payload.TSMS.Time
	}
	table := wire.Payload.Source.Table
	if table == "" {
		table = wire.Table
	}
	ev := Event{
		LSN:    lsn,
		TS:     ts,
		Op:     op,
		Table:  strings.TrimSpace(table),
		Before: wire.Payload.Before,
		After:  wire.Payload.After,
	}
	ev.PK = p.pkFor(nil, wire.Key, ev.Before, ev.After)
	if err := ev.Validate(); err != nil {
		return Event{}, err
	}
	return ev, nil
}

func decodeJSON(line []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}

// pkFor picks the first usable primary key from explicit fields or from the
// row images, in preference order.
func (p Parser) pkFor(explicit []any, images ...Record) string {
	for _, value := range explicit {
		if s := ScalarString(value); s != "" {
			return s
		}
	}
	candidates := []string{p.PKField}
	if p.PKField == "" {
		candidates = []string{"id", "pk", "key", "_id"}
	}
	for _, rec := range images {
		if rec == nil {
			continue
		}
		for _, name := range candidates {
			if value, exists := rec[name]; exists {
				if s := ScalarString(value); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// Decoder streams events from an NDJSON input.
type Decoder struct {
	sc     *bufio.Scanner
	parser Parser
	line   int
}

// NewDecoder returns a decoder that reads up to 16 MiB per event line.
func NewDecoder(r io.Reader, parser Parser) *Decoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	return &Decoder{sc: sc, parser: parser}
}

// Next returns the next event, or io.EOF at the end of the stream.
func (d *Decoder) Next() (Event, error) {
	for d.sc.Scan() {
		d.line++
		line := bytes.TrimSpace(d.sc.Bytes())
		if len(line) == 0 {
			continue
		}
		ev, err := d.parser.ParseLine(line)
		if err != nil {
			return Event{}, fmt.Errorf("line %d: %w", d.line, err)
		}
		ev.Line = d.line
		return ev, nil
	}
	if err := d.sc.Err(); err != nil {
		return Event{}, err
	}
	return Event{}, io.EOF
}

// All reads every event remaining in the stream.
func (d *Decoder) All() ([]Event, error) {
	var events []Event
	for {
		ev, err := d.Next()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
}

// flexUint64 accepts JSON numbers, numeric strings, and Postgres "hi/lo" LSN
// strings.
type flexUint64 uint64

func (f *flexUint64) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "null" || text == `""` {
		*f = 0
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
	}
	if text == "" {
		*f = 0
		return nil
	}
	if i := strings.IndexByte(text, '/'); i >= 0 {
		hi, herr := strconv.ParseUint(text[:i], 16, 32)
		lo, lerr := strconv.ParseUint(text[i+1:], 16, 32)
		if herr == nil && lerr == nil {
			*f = flexUint64(hi<<32 | lo)
			return nil
		}
	}
	n, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid lsn %q", text)
	}
	*f = flexUint64(n)
	return nil
}

// flexTime accepts RFC 3339 strings and epoch numbers in seconds,
// milliseconds, microseconds, or nanoseconds.
type flexTime struct {
	time.Time
}

func (f *flexTime) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "null" {
		f.Time = time.Time{}
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		t, err := parseTimestamp(s)
		if err != nil {
			return err
		}
		f.Time = t
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return fmt.Errorf("invalid timestamp %s", text)
	}
	if integer, err := number.Int64(); err == nil {
		f.Time = epochToTime(float64(integer))
		return nil
	}
	value, err := number.Float64()
	if err != nil {
		return fmt.Errorf("invalid timestamp %s", text)
	}
	f.Time = epochToTime(value)
	return nil
}

func parseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return epochToTime(float64(n)), nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", s)
}

func epochToTime(n float64) time.Time {
	switch {
	case n <= 0:
		return time.Time{}
	case n >= 1e17:
		return time.Unix(0, int64(n))
	case n >= 1e14:
		return time.UnixMicro(int64(n))
	case n >= 1e11:
		return time.UnixMilli(int64(n))
	case n >= 1e8:
		return time.Unix(int64(n), 0)
	default:
		return time.Time{}
	}
}

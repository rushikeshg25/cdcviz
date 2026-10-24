package render

import (
	"io"
	"strconv"
	"testing"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

func BenchmarkTimelineManyRecords(b *testing.B) {
	events := make([]event.Event, 10000)
	for i := range events {
		events[i] = event.Event{Op: event.Insert, Table: "records", PK: strconv.Itoa(i)}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Timeline(events, io.Discard, TimelineOptions{Width: 100, MaxRows: 20})
	}
}

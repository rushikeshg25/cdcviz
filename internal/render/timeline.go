package render

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

// TimelineOptions controls layout. Zero values select defaults.
type TimelineOptions struct {
	Width   int
	MaxRows int
	Theme   *Theme
}

// Timeline draws one lifecycle bar per table/primary key. Events are placed on
// a shared sequence axis, so gaps and bursts are visible at a glance.
func Timeline(events []event.Event, out io.Writer, opts TimelineOptions) {
	theme := opts.Theme
	if theme == nil {
		theme = NewTheme(false)
	}
	width := opts.Width
	if width <= 0 {
		width = 100
	}
	if width < 40 {
		width = 40
	}
	maxRows := opts.MaxRows
	if maxRows <= 0 {
		maxRows = 20
	}

	if len(events) == 0 {
		fmt.Fprintln(out, "no events")
		return
	}

	type group struct {
		key   string
		count int
	}
	groups := map[event.RecordID]*group{}
	var order []event.RecordID
	for _, ev := range events {
		g := groups[ev.Identity()]
		if g == nil {
			g = &group{key: ev.Key()}
			groups[ev.Identity()] = g
			order = append(order, ev.Identity())
		}
		g.count++
	}

	labelWidth := 12
	for _, key := range order {
		if n := len([]rune(groups[key].key)); n > labelWidth {
			labelWidth = n
		}
	}
	if labelWidth > 30 {
		labelWidth = 30
	}
	const suffixWidth = 12
	const minBarWidth = 10
	// Keep the whole row inside the requested width even when a key is long and
	// --width is small; truncate the label rather than overflowing the terminal.
	if labelWidth+minBarWidth+suffixWidth > width {
		labelWidth = width - minBarWidth - suffixWidth
	}
	barWidth := width - labelWidth - suffixWidth

	start, end := timeRange(events)
	fmt.Fprintf(out, "%s  %d events | %d records | %s\n\n",
		theme.Bold("Timeline"), len(events), len(order), formatSpan(start, end))

	// Busiest records first, then stable by key.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := groups[order[i]], groups[order[j]]
		if a.count != b.count {
			return a.count > b.count
		}
		if order[i].Table != order[j].Table {
			return order[i].Table < order[j].Table
		}
		return order[i].PK < order[j].PK
	})

	columns := make(map[event.RecordID][]event.Op, len(order))
	for i := range events {
		ev := events[i]
		column := 0
		if len(events) > 1 {
			column = i * (barWidth - 1) / (len(events) - 1)
		}
		marks := columns[ev.Identity()]
		if marks == nil {
			marks = make([]event.Op, barWidth)
		}
		// Later events overwrite earlier ones when they share a column; the
		// event count still accounts for every event.
		marks[column] = ev.Op
		columns[ev.Identity()] = marks
	}

	rendered := 0
	for _, key := range order {
		if rendered == maxRows {
			break
		}
		rendered++
		marks := columns[key]
		bar := ""
		for _, op := range marks {
			if op == "" {
				bar += theme.Dim(".")
				continue
			}
			bar += theme.ColorOp(op, OpSymbol(op))
		}
		fmt.Fprintf(out, "%-*s %s %s\n",
			labelWidth, truncate(groups[key].key, labelWidth),
			bar,
			theme.Dim(fmt.Sprintf("%3d events", groups[key].count)))
	}
	if len(order) > rendered {
		fmt.Fprintf(out, "%s\n", theme.Dim(fmt.Sprintf("... %d more records not shown (raise --max-rows)", len(order)-rendered)))
	}

	fmt.Fprintf(out, "\n%s  %s insert   %s update   %s delete   %s read\n",
		theme.Dim("Legend:"),
		theme.ColorOp(event.Insert, "+"),
		theme.ColorOp(event.Update, "~"),
		theme.ColorOp(event.Delete, "-"),
		theme.ColorOp(event.Read, "."))
}

func timeRange(events []event.Event) (time.Time, time.Time) {
	var first, last time.Time
	for _, ev := range events {
		if ev.TS.IsZero() {
			continue
		}
		if first.IsZero() || ev.TS.Before(first) {
			first = ev.TS
		}
		if last.IsZero() || ev.TS.After(last) {
			last = ev.TS
		}
	}
	return first, last
}

// formatSpan keeps timestamps compact for same-day streams and adds dates when
// a capture spans midnight.
func formatSpan(first, last time.Time) string {
	switch {
	case first.IsZero() && last.IsZero():
		return "-- -> --"
	case first.IsZero():
		return "-- -> " + event.FormatTime(last)
	case last.IsZero():
		return event.FormatTime(first) + " -> --"
	}
	if first.UTC().Format("2006-01-02") != last.UTC().Format("2006-01-02") {
		const layout = "2006-01-02 15:04:05"
		return first.UTC().Format(layout) + " -> " + last.UTC().Format(layout)
	}
	return event.FormatTime(first) + " -> " + event.FormatTime(last)
}

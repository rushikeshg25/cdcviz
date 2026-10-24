package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

// ReplayOptions controls animated playback. Speed is events per second; zero
// disables sleeping.
type ReplayOptions struct {
	Speed   float64
	Limit   int
	Animate bool
	Theme   *Theme
	// Sleep is injectable for tests. It defaults to time.Sleep.
	Sleep func(time.Duration)
}

// Replay prints events one at a time, optionally rewriting a single terminal
// line. In non-animated mode every event is a normal line, which keeps piped
// output readable.
func Replay(events []event.Event, out io.Writer, opts ReplayOptions) {
	theme := opts.Theme
	if theme == nil {
		theme = NewTheme(false)
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	if len(events) == 0 {
		fmt.Fprintln(out, "no events")
		return
	}

	shown := events
	if opts.Limit > 0 && len(events) > opts.Limit {
		shown = events[:opts.Limit]
	}
	total := len(events)
	indexWidth := len(strconv.Itoa(total))
	var interval time.Duration
	if opts.Speed > 0 {
		interval = time.Duration(float64(time.Second) / opts.Speed)
	}

	counts := map[event.Op]int{}
	for i, ev := range shown {
		counts[ev.Op]++
		line := fmt.Sprintf("[%*d/%d] %s  %s %-6s  %-26s %s",
			indexWidth, i+1, total,
			event.FormatTime(ev.TS),
			theme.ColorOp(ev.Op, OpSymbol(ev.Op)),
			theme.ColorOp(ev.Op, strings.ToUpper(ev.Op.String())),
			truncate(ev.Key(), 26),
			truncate(ev.Describe(), 90))
		if opts.Animate {
			fmt.Fprintf(out, "\r\x1b[2K%s", line)
		} else {
			fmt.Fprintln(out, line)
		}
		if interval > 0 {
			sleep(interval)
		}
	}
	if opts.Animate {
		fmt.Fprintln(out)
	}

	limitNote := ""
	if len(shown) < total {
		limitNote = fmt.Sprintf(" (limit %d)", len(shown))
	}
	fmt.Fprintf(out, "\n%s\n", theme.Dim(fmt.Sprintf(
		"replayed %d/%d events%s | insert %d, update %d, delete %d, read %d",
		len(shown), total, limitNote,
		counts[event.Insert], counts[event.Update], counts[event.Delete], counts[event.Read])))
}

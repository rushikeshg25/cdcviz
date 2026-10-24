package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

// Record renders the full history of one table/primary key as a vertical
// timeline with before/after field changes.
func Record(events []event.Event, table, pk string, out io.Writer, theme *Theme) error {
	if theme == nil {
		theme = NewTheme(false)
	}
	var history []event.Event
	for _, ev := range events {
		if ev.Table == table && ev.PK == pk {
			history = append(history, ev)
		}
	}
	if len(history) == 0 {
		return fmt.Errorf("record %s/%s not found", table, pk)
	}

	first, last := timeRange(history)
	fmt.Fprintf(out, "%s  %d events | %s -> %s\n\n",
		theme.Bold(table+"/"+pk), len(history), formatRange(first), formatRange(last))

	for i, ev := range history {
		fmt.Fprintf(out, "  #%-2d %s  %s %-6s  lsn=%s\n",
			i+1,
			event.FormatTime(ev.TS),
			theme.ColorOp(ev.Op, OpSymbol(ev.Op)),
			theme.ColorOp(ev.Op, strings.ToUpper(ev.Op.String())),
			formatLSN(ev.LSN))
		changes := ev.Changes()
		if len(changes) == 0 {
			fmt.Fprintf(out, "        %s\n", theme.Dim("(no field changes)"))
		}
		for _, change := range changes {
			fmt.Fprintf(out, "        %s\n", truncate(change, 120))
		}
		if i < len(history)-1 {
			fmt.Fprintln(out)
		}
	}
	return nil
}

func formatLSN(lsn uint64) string {
	if lsn == 0 {
		return "--"
	}
	return strconv.FormatUint(lsn, 10)
}

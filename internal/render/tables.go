package render

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/rushikeshg25/cdcviz/internal/stats"
)

// Tables renders a per-table activity table with a totals row.
func Tables(summary stats.Summary, out io.Writer) {
	if summary.Events == 0 {
		fmt.Fprintln(out, "no events")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TABLE\tEVENTS\tINSERT\tUPDATE\tDELETE\tREAD\tKEYS\tANOMALIES")
	for _, name := range summary.TableNames() {
		t := summary.Tables[name]
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			name, t.Events, t.Inserts, t.Updates, t.Deletes, t.Reads, t.Keys, t.Anomalies)
	}
	fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		summary.Events,
		summary.Ops["insert"],
		summary.Ops["update"],
		summary.Ops["delete"],
		summary.Ops["read"],
		summary.Keys,
		summary.Anomalies)
	tw.Flush()
}

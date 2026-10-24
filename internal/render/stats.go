package render

import (
	"fmt"
	"io"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/stats"
)

// Stats renders aggregate counters and anomaly frequencies.
func Stats(summary stats.Summary, out io.Writer, theme *Theme) {
	if theme == nil {
		theme = NewTheme(false)
	}
	if summary.Events == 0 {
		fmt.Fprintln(out, "no events")
		return
	}

	fmt.Fprintln(out, theme.Bold("Summary"))
	fmt.Fprintf(out, "  %-12s %d\n", "events", summary.Events)
	fmt.Fprintf(out, "  %-12s %d\n", "records", summary.Keys)
	fmt.Fprintf(out, "  %-12s %d\n", "tables", len(summary.Tables))
	fmt.Fprintf(out, "  %-12s %s\n", "window", window(summary))
	fmt.Fprintf(out, "  %-12s %.2f events/sec\n", "throughput", summary.EventsPerSecond)

	fmt.Fprintf(out, "\n%s\n", theme.Bold("Operations"))
	ops := summary.Ops
	fmt.Fprintf(out, "  %-12s %d\n", "insert", ops["insert"])
	fmt.Fprintf(out, "  %-12s %d\n", "update", ops["update"])
	fmt.Fprintf(out, "  %-12s %d\n", "delete", ops["delete"])
	fmt.Fprintf(out, "  %-12s %d\n", "read", ops["read"])

	fmt.Fprintf(out, "\n%s  %d\n", theme.Bold("Anomalies"), summary.Anomalies)
	if summary.Anomalies > 0 {
		fmt.Fprintf(out, "  %-12s %d\n", "error", summary.AnomaliesBySeverity["error"])
		fmt.Fprintf(out, "  %-12s %d\n", "warning", summary.AnomaliesBySeverity["warning"])
		fmt.Fprintf(out, "  %-12s %d\n", "info", summary.AnomaliesBySeverity["info"])
		for _, count := range summary.KindCounts() {
			fmt.Fprintf(out, "  %-24s %d\n", count.Kind, count.Count)
		}
	}
}

func window(summary stats.Summary) string {
	if summary.FirstTS.IsZero() || summary.LastTS.IsZero() {
		return "--"
	}
	return fmt.Sprintf("%s -> %s (%.2fs)",
		summary.FirstTS.UTC().Format(time.DateTime),
		summary.LastTS.UTC().Format(time.DateTime),
		summary.DurationSeconds)
}

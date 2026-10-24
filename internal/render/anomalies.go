package render

import (
	"fmt"
	"io"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
)

// Anomalies renders findings in input order, with a severity rollup.
func Anomalies(findings []anomaly.Anomaly, out io.Writer, theme *Theme) {
	if theme == nil {
		theme = NewTheme(false)
	}
	if len(findings) == 0 {
		fmt.Fprintln(out, theme.paint(green, "no anomalies detected"))
		return
	}

	errors, warnings, infos := 0, 0, 0
	for _, finding := range findings {
		switch finding.Severity {
		case anomaly.Error:
			errors++
		case anomaly.Warning:
			warnings++
		default:
			infos++
		}
	}
	fmt.Fprintf(out, "%s  %d findings (%d error, %d warning, %d info)\n\n",
		theme.Bold("Anomalies"), len(findings), errors, warnings, infos)

	for _, finding := range findings {
		fmt.Fprintf(out, "%s  %-24s %s  [%s]  %s\n",
			theme.Dim(fmt.Sprintf("line %-4d", finding.Line)),
			truncate(finding.Key(), 24),
			theme.Dim(fmt.Sprintf("lsn=%-9s", formatLSN(finding.LSN))),
			theme.ColorSeverity(finding.Severity, string(finding.Severity)),
			finding.Kind)
		if finding.Message != "" {
			fmt.Fprintf(out, "          %s\n", finding.Message)
		}
	}
}

// Package render draws the terminal views: timelines, tables, record
// histories, anomaly lists, aggregate stats, and animated replay.
package render

import (
	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/event"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
)

// Theme applies ANSI styling when enabled.
type Theme struct {
	Enabled bool
}

// NewTheme returns a theme that either emits ANSI escapes or plain text.
func NewTheme(enabled bool) *Theme { return &Theme{Enabled: enabled} }

func (t *Theme) paint(code, s string) string {
	if t == nil || !t.Enabled {
		return s
	}
	return code + s + reset
}

// Bold renders emphasized text.
func (t *Theme) Bold(s string) string { return t.paint(bold, s) }

// Dim renders secondary text.
func (t *Theme) Dim(s string) string { return t.paint(dim, s) }

// OpSymbol returns the single-character marker for an operation.
func OpSymbol(op event.Op) string {
	switch op {
	case "insert":
		return "+"
	case "update":
		return "~"
	case "delete":
		return "-"
	case "read":
		return "."
	default:
		return "?"
	}
}

// ColorOp renders an operation name in its colour.
func (t *Theme) ColorOp(op event.Op, s string) string {
	switch op {
	case "insert":
		return t.paint(green, s)
	case "update":
		return t.paint(yellow, s)
	case "delete":
		return t.paint(red, s)
	case "read":
		return t.paint(cyan, s)
	default:
		return s
	}
}

// ColorSeverity renders a severity label in its colour.
func (t *Theme) ColorSeverity(severity anomaly.Severity, s string) string {
	switch severity {
	case anomaly.Error:
		return t.paint(red, s)
	case anomaly.Warning:
		return t.paint(yellow, s)
	case anomaly.Info:
		return t.paint(cyan, s)
	default:
		return s
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "~"
}

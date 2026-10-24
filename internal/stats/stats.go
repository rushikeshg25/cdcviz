// Package stats aggregates a CDC stream into per-table and per-operation
// summaries for the stats and tables views.
package stats

import (
	"sort"
	"time"

	"github.com/rushikeshg25/cdcviz/internal/anomaly"
	"github.com/rushikeshg25/cdcviz/internal/event"
)

// Table summarizes one table's events.
type Table struct {
	Events    int `json:"events"`
	Inserts   int `json:"inserts"`
	Updates   int `json:"updates"`
	Deletes   int `json:"deletes"`
	Reads     int `json:"reads"`
	Keys      int `json:"keys"`
	Anomalies int `json:"anomalies"`
}

// Summary is the aggregate view of a stream.
type Summary struct {
	Events              int               `json:"events"`
	Keys                int               `json:"keys"`
	Tables              map[string]*Table `json:"tables"`
	Ops                 map[string]int    `json:"ops"`
	FirstTS             time.Time         `json:"first_ts"`
	LastTS              time.Time         `json:"last_ts"`
	DurationSeconds     float64           `json:"duration_seconds"`
	EventsPerSecond     float64           `json:"events_per_second"`
	Anomalies           int               `json:"anomalies"`
	AnomaliesByKind     map[string]int    `json:"anomalies_by_kind"`
	AnomaliesBySeverity map[string]int    `json:"anomalies_by_severity"`
}

// Compute aggregates events and the anomalies detected in them.
func Compute(events []event.Event, anomalies []anomaly.Anomaly) Summary {
	summary := Summary{
		Tables:              map[string]*Table{},
		Ops:                 map[string]int{},
		AnomaliesByKind:     map[string]int{},
		AnomaliesBySeverity: map[string]int{},
	}
	globalKeys := map[string]bool{}
	tableKeys := map[string]map[string]bool{}

	for _, ev := range events {
		table := summary.Tables[ev.Table]
		if table == nil {
			table = &Table{}
			summary.Tables[ev.Table] = table
		}
		table.Events++
		switch ev.Op {
		case event.Insert:
			table.Inserts++
		case event.Update:
			table.Updates++
		case event.Delete:
			table.Deletes++
		case event.Read:
			table.Reads++
		}
		summary.Events++
		summary.Ops[ev.Op.String()]++
		globalKeys[ev.Key()] = true
		if tableKeys[ev.Table] == nil {
			tableKeys[ev.Table] = map[string]bool{}
		}
		tableKeys[ev.Table][ev.PK] = true

		if !ev.TS.IsZero() {
			if summary.FirstTS.IsZero() || ev.TS.Before(summary.FirstTS) {
				summary.FirstTS = ev.TS
			}
			if summary.LastTS.IsZero() || ev.TS.After(summary.LastTS) {
				summary.LastTS = ev.TS
			}
		}
	}

	for name, keys := range tableKeys {
		if table := summary.Tables[name]; table != nil {
			table.Keys = len(keys)
		}
	}

	for _, a := range anomalies {
		summary.Anomalies++
		summary.AnomaliesByKind[string(a.Kind)]++
		summary.AnomaliesBySeverity[string(a.Severity)]++
		if table := summary.Tables[a.Table]; table != nil {
			table.Anomalies++
		}
	}

	summary.Keys = len(globalKeys)
	if !summary.FirstTS.IsZero() && !summary.LastTS.IsZero() {
		summary.DurationSeconds = summary.LastTS.Sub(summary.FirstTS).Seconds()
		if summary.DurationSeconds > 0 {
			summary.EventsPerSecond = float64(summary.Events) / summary.DurationSeconds
		}
	}
	return summary
}

// TableNames returns table names in sorted order.
func (s Summary) TableNames() []string {
	names := make([]string, 0, len(s.Tables))
	for name := range s.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// KindCounts returns anomaly kinds sorted by count (descending) then name.
func (s Summary) KindCounts() []KindCount {
	out := make([]KindCount, 0, len(s.AnomaliesByKind))
	for kind, count := range s.AnomaliesByKind {
		out = append(out, KindCount{Kind: kind, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// KindCount pairs an anomaly kind with its frequency.
type KindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

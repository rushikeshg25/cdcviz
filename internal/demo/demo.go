// Package demo provides a deterministic CDC scenario so cdcviz can be tried
// without a real database. The stream intentionally contains eight
// representative integrity problems, annotated below.
package demo

import (
	"time"

	"github.com/rushikeshg25/cdcviz/internal/event"
)

// Events returns a 17-event scenario across users and orders.
func Events() []event.Event {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	var events []event.Event

	add := func(seconds int, lsn uint64, op event.Op, table, pk string, before, after event.Record) {
		events = append(events, event.Event{
			LSN:    lsn,
			TS:     base.Add(time.Duration(seconds) * time.Second),
			Op:     op,
			Table:  table,
			PK:     pk,
			Before: before,
			After:  after,
		})
	}

	add(0, 100, event.Insert, "users", "1", nil, event.Record{
		"id": 1, "name": "Ada", "email": "ada@example.com",
	})
	add(2, 103, event.Update, "users", "1",
		event.Record{"id": 1, "name": "Ada", "email": "ada@example.com"},
		event.Record{"id": 1, "name": "Ada Lovelace", "email": "ada@example.com"})
	add(4, 107, event.Insert, "users", "2", nil, event.Record{"id": 2, "name": "Grace"})
	add(5, 109, event.Insert, "orders", "10", nil, event.Record{
		"id": 10, "user_id": 1, "status": "pending",
	})

	// A change, immediately followed by an exact redelivery of it: duplicate-event.
	emailChange := event.Event{
		LSN:    114,
		TS:     base.Add(7 * time.Second),
		Op:     event.Update,
		Table:  "users",
		PK:     "1",
		Before: event.Record{"id": 1, "name": "Ada Lovelace", "email": "ada@example.com"},
		After:  event.Record{"id": 1, "name": "Ada Lovelace", "email": "ada@lovelace.dev"},
	}
	emailChange.Line = len(events) + 1
	events = append(events, emailChange)
	duplicate := emailChange
	duplicate.Line = len(events) + 1
	events = append(events, duplicate)

	add(9, 118, event.Update, "orders", "10",
		event.Record{"id": 10, "status": "pending"},
		event.Record{"id": 10, "status": "paid"})
	add(11, 121, event.Delete, "users", "2", event.Record{"id": 2, "name": "Grace"}, nil)
	add(13, 125, event.Update, "orders", "10",
		event.Record{"id": 10, "status": "paid"},
		event.Record{"id": 10, "status": "shipped"})

	// A delete for a row that was never seen in the stream: delete-before-insert.
	add(15, 128, event.Delete, "orders", "11", event.Record{"id": 11, "status": "open"}, nil)
	// Traffic for a deleted row, then its reappearance: update-after-delete and resurrection.
	add(16, 130, event.Update, "users", "2",
		event.Record{"id": 2},
		event.Record{"id": 2, "name": "Grace Hopper"})
	add(18, 133, event.Insert, "users", "2", nil, event.Record{"id": 2, "name": "Grace Hopper"})

	// A second insert for a live row: duplicate-insert.
	add(20, 137, event.Insert, "users", "1", nil, event.Record{
		"id": 1, "name": "Ada Lovelace", "email": "ada@lovelace.dev",
	})
	// An LSN that jumps backwards: out-of-order-lsn.
	add(21, 120, event.Update, "orders", "10",
		event.Record{"id": 10, "status": "shipped"},
		event.Record{"id": 10, "status": "delivered"})
	// A timestamp older than the same key's previous event: stale-timestamp.
	add(18, 140, event.Update, "users", "1",
		event.Record{"id": 1, "name": "Ada Lovelace"},
		event.Record{"id": 1, "name": "Ada L."})
	// before == after: no-op-update.
	add(22, 143, event.Update, "users", "1",
		event.Record{"id": 1, "name": "Ada L."},
		event.Record{"id": 1, "name": "Ada L."})

	add(23, 150, event.Read, "orders", "10", nil, event.Record{
		"id": 10, "user_id": 1, "status": "delivered",
	})

	for i := range events {
		events[i].Line = i + 1
	}
	return events
}

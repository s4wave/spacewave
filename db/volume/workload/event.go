//go:build !js

package workload

import (
	"io"
	"os"

	"github.com/pkg/errors"
	exptrace "golang.org/x/exp/trace"
)

// Event is a Record with the trace time and goroutine that emitted it.
type Event struct {
	// Time is the trace timestamp in nanoseconds.
	Time int64
	// Goroutine is the emitting goroutine. GoScript reports one goroutine for
	// every event, so replay orders by Time and correlates by Record IDs.
	Goroutine int64
	// Record is the logical operation.
	Record Record
}

// Extract reads a Go execution trace and returns its workload records in
// trace order, skipping every other event.
func Extract(r io.Reader) ([]Event, error) {
	rd, err := exptrace.NewReader(r)
	if err != nil {
		return nil, errors.Wrap(err, "open trace")
	}
	var out []Event
	for {
		// Read the next event until the trace ends.
		ev, err := rd.ReadEvent()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, errors.Wrap(err, "read trace")
		}

		// Keep only workload log events.
		if ev.Kind() != exptrace.EventLog {
			continue
		}
		log := ev.Log()
		if log.Category != Category {
			continue
		}
		rec, err := ParseRecord(log.Message)
		if err != nil {
			return nil, err
		}
		out = append(out, Event{Time: int64(ev.Time()), Goroutine: int64(ev.Goroutine()), Record: rec})
	}
}

// ReadTrace prepares a replay of the Go execution trace file at path.
func ReadTrace(path string) (*Replay, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	events, err := Extract(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	records := make([]Record, len(events))
	for i, ev := range events {
		records[i] = ev.Record
	}
	return NewReplay(records)
}

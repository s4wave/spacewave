package plugin_host_logs

import (
	"testing"
	"time"
)

func TestHubAssignsMonotonicSequencesAndBoundsRetainedHistory(t *testing.T) {
	hub := NewHub(
		WithRetainedEventLimit(3),
		WithClock(func() time.Time { return time.Unix(123, 0) }),
	)

	for i := uint64(1); i <= 5; i++ {
		resp, err := hub.Emit(&StructuredLogEvent{
			PluginId: "plugin-a",
			Message:  "event",
		})
		if err != nil {
			t.Fatalf("Emit: %v", err)
		}
		if resp.GetSequence() != i {
			t.Fatalf("sequence = %d, want %d", resp.GetSequence(), i)
		}
		if resp.GetTimestamp() == nil {
			t.Fatalf("timestamp was not assigned")
		}
	}

	state := hub.Snapshot(nil, nil)
	got := eventSequences(state.GetEvents())
	want := []uint64{3, 4, 5}
	if !equalSequences(got, want) {
		t.Fatalf("retained sequences = %v, want %v", got, want)
	}
}

func TestHubEvaluatesStructuredLogFilters(t *testing.T) {
	hub := NewHub(WithRetainedEventLimit(10))
	events := []*StructuredLogEvent{
		{
			PluginId:    "runner",
			InstanceKey: "one",
			Stream:      StructuredLogStream_STRUCTURED_LOG_STREAM_STDERR,
			Level:       StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN,
			Message:     "Retrying build",
			Fields: map[string]string{
				"component": "executor",
			},
		},
		{
			PluginId: "runner",
			Stream:   StructuredLogStream_STRUCTURED_LOG_STREAM_STDOUT,
			Level:    StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN,
			Message:  "Retrying build",
			Fields: map[string]string{
				"component": "executor",
			},
		},
		{
			PluginId: "runner",
			Stream:   StructuredLogStream_STRUCTURED_LOG_STREAM_STDERR,
			Level:    StructuredLogLevel_STRUCTURED_LOG_LEVEL_INFO,
			Message:  "Retrying build",
			Fields: map[string]string{
				"component": "executor",
			},
		},
		{
			PluginId: "other",
			Stream:   StructuredLogStream_STRUCTURED_LOG_STREAM_STDERR,
			Level:    StructuredLogLevel_STRUCTURED_LOG_LEVEL_ERROR,
			Message:  "Retrying build",
			Fields: map[string]string{
				"component": "executor",
			},
		},
		{
			PluginId: "runner",
			Stream:   StructuredLogStream_STRUCTURED_LOG_STREAM_STDERR,
			Level:    StructuredLogLevel_STRUCTURED_LOG_LEVEL_ERROR,
			Message:  "Done",
			Fields: map[string]string{
				"component": "executor",
				"detail":    "retry queue drained",
			},
		},
	}
	for _, event := range events {
		if _, err := hub.Emit(event); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}

	state := hub.Snapshot(&StructuredLogFilter{
		PluginIds: []string{"runner"},
		Streams: []StructuredLogStream{
			StructuredLogStream_STRUCTURED_LOG_STREAM_STDERR,
		},
		MinLevel:   StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN,
		SearchText: "RETRY",
		Fields: map[string]string{
			"component": "executor",
		},
	}, nil)

	got := eventSequences(state.GetEvents())
	want := []uint64{1, 5}
	if !equalSequences(got, want) {
		t.Fatalf("filtered sequences = %v, want %v", got, want)
	}
}

func TestHubRangeTailLimitAndDroppedCount(t *testing.T) {
	hub := NewHub(WithRetainedEventLimit(10))
	for range 5 {
		if _, err := hub.Emit(&StructuredLogEvent{
			PluginId: "plugin-a",
			Message:  "event",
		}); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}

	state := hub.Snapshot(nil, &StructuredLogRange{
		Limit: 2,
		Tail:  true,
	})
	got := eventSequences(state.GetEvents())
	want := []uint64{4, 5}
	if !equalSequences(got, want) {
		t.Fatalf("tail sequences = %v, want %v", got, want)
	}
	if state.GetDroppedEventCount() != 3 {
		t.Fatalf("dropped count = %d, want 3", state.GetDroppedEventCount())
	}
}

func TestHubFastPathSkipsInactiveFollowViews(t *testing.T) {
	hub := NewHub(WithRetainedEventLimit(0))

	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
		t.Fatalf("Emit inactive: %v", err)
	}
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("inactive retained events = %d, want 0", got)
	}

	view := hub.OpenView(
		&StructuredLogFilter{PluginIds: []string{"runner"}},
		&StructuredLogRange{Follow: true},
	)
	defer view.Release()

	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "other"}); err != nil {
		t.Fatalf("Emit non-matching: %v", err)
	}
	select {
	case <-view.Updates():
		t.Fatalf("view updated for non-matching event")
	default:
	}

	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
		t.Fatalf("Emit matching: %v", err)
	}
	select {
	case <-view.Updates():
	default:
		t.Fatalf("view did not update for matching followed event")
	}

	got := eventSequences(view.Snapshot().GetEvents())
	want := []uint64{3}
	if !equalSequences(got, want) {
		t.Fatalf("followed view sequences = %v, want %v", got, want)
	}
}

func eventSequences(events []*StructuredLogEvent) []uint64 {
	seq := make([]uint64, len(events))
	for i, event := range events {
		seq[i] = event.GetSequence()
	}
	return seq
}

func equalSequences(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package plugin_host_logs

import (
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestHubAssignsMonotonicSequencesAndBoundsRetainedHistory(t *testing.T) {
	// Open a retained log view with a deterministic event clock.
	hub := NewHub(
		WithRetainedEventLimit(3),
		WithClock(func() time.Time { return time.Unix(123, 0) }),
	)
	view := hub.OpenView(nil, nil)
	defer view.Release()

	// Emit more events than the retained history can hold.
	for i := uint64(1); i <= 5; i++ {
		resp, err := hub.Emit(&StructuredLogEvent{
			PluginId: "plugin-a",
			Message:  "event",
		})
		if err != nil {
			t.Fatalf("Emit: %v", err)
		}

		// Verify the sequence and timestamp assigned to this event.
		if resp.GetSequence() != i {
			t.Fatalf("sequence = %d, want %d", resp.GetSequence(), i)
		}
		if resp.GetTimestamp() == nil {
			t.Fatalf("timestamp was not assigned")
		}
	}

	// Verify that only the newest three events remain in history.
	state := hub.Snapshot(nil, nil)
	got := eventSequences(state.GetEvents())
	want := []uint64{3, 4, 5}
	if !equalSequences(got, want) {
		t.Fatalf("retained sequences = %v, want %v", got, want)
	}
}

func TestHubEvaluatesStructuredLogFilters(t *testing.T) {
	// Open a retained log view for structured filtering.
	hub := NewHub(WithRetainedEventLimit(10))
	view := hub.OpenView(nil, nil)
	defer view.Release()

	// Emit events that vary the fields used by the filter.
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

	// Read the history through the combined structured log filter.
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

	// Verify that only the matching events remain in the snapshot.
	got := eventSequences(state.GetEvents())
	want := []uint64{1, 5}
	if !equalSequences(got, want) {
		t.Fatalf("filtered sequences = %v, want %v", got, want)
	}
}

func TestHubRangeTailLimitAndDroppedCount(t *testing.T) {
	// Open a retained log view for tail range queries.
	hub := NewHub(WithRetainedEventLimit(10))
	view := hub.OpenView(nil, nil)
	defer view.Release()

	// Emit five events for the tail range to select.
	for range 5 {
		if _, err := hub.Emit(&StructuredLogEvent{
			PluginId: "plugin-a",
			Message:  "event",
		}); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}

	// Verify the newest two events and the omitted event count.
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

func TestHubRetainsHistoryOnlyWhileViewsAreOpen(t *testing.T) {
	// Create a hub whose history requires an open view.
	hub := NewHub(WithRetainedEventLimit(10))

	// Verify that emitting without a view advances the sequence without retention.
	resp, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"})
	if err != nil {
		t.Fatalf("Emit without view: %v", err)
	}
	if resp.GetSequence() != 1 {
		t.Fatalf("sequence without view = %d, want 1", resp.GetSequence())
	}
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("retained events without view = %d, want 0", got)
	}

	// Open two views that share the retained history.
	view := hub.OpenView(nil, nil)
	secondView := hub.OpenView(nil, nil)

	// Emit events and verify retention while both views remain open.
	for range 2 {
		if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
			t.Fatalf("Emit with view: %v", err)
		}
	}
	got := eventSequences(hub.Snapshot(nil, nil).GetEvents())
	want := []uint64{2, 3}
	if !equalSequences(got, want) {
		t.Fatalf("retained sequences with views = %v, want %v", got, want)
	}

	// Release one view and verify that the other retains the history.
	view.Release()
	got = eventSequences(hub.Snapshot(nil, nil).GetEvents())
	if !equalSequences(got, want) {
		t.Fatalf("retained sequences with second view = %v, want %v", got, want)
	}

	// Release the last view and require an empty retained history.
	secondView.Release()
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("retained events after last release = %d, want 0", got)
	}

	// Verify that event numbering continues after retention stops.
	resp, err = hub.Emit(&StructuredLogEvent{PluginId: "runner"})
	if err != nil {
		t.Fatalf("Emit after last release: %v", err)
	}
	if resp.GetSequence() != 4 {
		t.Fatalf("sequence after last release = %d, want 4", resp.GetSequence())
	}

	// Reopen a view and require an empty initial history.
	reopenedView := hub.OpenView(nil, nil)
	defer reopenedView.Release()
	if got := len(reopenedView.Snapshot().GetEvents()); got != 0 {
		t.Fatalf("retained events after reopening = %d, want 0", got)
	}
}

func TestHubFastPathSkipsInactiveFollowViews(t *testing.T) {
	// Create a hub with no retained history capacity.
	hub := NewHub(WithRetainedEventLimit(0))

	// Emit without a view and require no retained events.
	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
		t.Fatalf("Emit inactive: %v", err)
	}
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("inactive retained events = %d, want 0", got)
	}

	// Open a following view that selects the runner plugin.
	view := hub.OpenView(
		&StructuredLogFilter{PluginIds: []string{"runner"}},
		&StructuredLogRange{Follow: true},
	)
	defer view.Release()

	// Emit an unrelated event and require no view update.
	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "other"}); err != nil {
		t.Fatalf("Emit non-matching: %v", err)
	}
	select {
	case <-view.Updates():
		t.Fatalf("view updated for non-matching event")
	default:
	}

	// Emit a matching event and require a view update.
	if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
		t.Fatalf("Emit matching: %v", err)
	}
	select {
	case <-view.Updates():
	default:
		t.Fatalf("view did not update for matching followed event")
	}

	// Verify that the following view exposes its matching event.
	got := eventSequences(view.Snapshot().GetEvents())
	want := []uint64{3}
	if !equalSequences(got, want) {
		t.Fatalf("followed view sequences = %v, want %v", got, want)
	}
}

func TestViewUpdatesCoalesceAndCloseOnRelease(t *testing.T) {
	// Open a log view and capture its update channel.
	hub := NewHub()
	view := hub.OpenView(nil, nil)
	ch := view.Updates()

	// Change the view twice before consuming its update.
	view.Set(nil, nil)
	view.Set(nil, nil)

	// Verify that the two changes produce one coalesced update.
	select {
	case <-ch:
	default:
		t.Fatal("view update was not signaled")
	}
	select {
	case <-ch:
		t.Fatal("view updates should coalesce")
	default:
	}

	// Release the view and require its update channel to close.
	view.Release()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("view update channel should be closed")
		}
	default:
		t.Fatal("view update channel did not close on release")
	}
}

func TestHubBroadcastsOnStateChanges(t *testing.T) {
	// Create a hub and a helper for checking its state broadcasts.
	hub := NewHub()
	assertHubBroadcasts := func(name string, mutate func()) {
		// Mark the broadcast assertion as a test helper.
		t.Helper()

		// Subscribe to the next hub state change.
		locked := hub.bcast.Lock()
		ch := locked.WaitCh()
		locked.Unlock()

		// Apply the mutation and require a hub broadcast.
		mutate()
		select {
		case <-ch:
		default:
			t.Fatalf("%s did not broadcast", name)
		}
	}

	// Verify broadcasts for opening, emitting, changing, and releasing a view.
	var view *View
	assertHubBroadcasts("OpenView", func() {
		view = hub.OpenView(nil, nil)
	})
	assertHubBroadcasts("Emit", func() {
		if _, err := hub.Emit(&StructuredLogEvent{PluginId: "runner"}); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	})
	assertHubBroadcasts("Set", func() {
		view.Set(nil, &StructuredLogRange{Follow: true})
	})
	assertHubBroadcasts("Release", view.Release)
}

func TestHostLogrusHookCapturesEvents(t *testing.T) {
	// Attach a host log hook to a silent debug logger.
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	hub := NewHub(WithRetainedEventLimit(10))
	release := AttachHostLogrusHook(nil, log, hub)
	defer release()
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("warn hooks = %d, want 1", got)
	}

	// Open a retained view for the host log event.
	view := hub.OpenView(nil, nil)
	defer view.Release()

	// Emit a host warning with structured plugin fields.
	log.WithFields(logrus.Fields{
		"plugin-id":    "runner",
		"instance-key": "main",
		"attempt":      2,
	}).Warn("host captured")

	// Verify the captured host event and its structured fields.
	events := hub.Snapshot(nil, nil).GetEvents()
	if len(events) != 1 {
		t.Fatalf("captured events = %d, want 1", len(events))
	}
	event := events[0]
	if event.GetPluginId() != "runner" {
		t.Fatalf("plugin id = %q, want runner", event.GetPluginId())
	}
	if event.GetInstanceKey() != "main" {
		t.Fatalf("instance key = %q, want main", event.GetInstanceKey())
	}
	if event.GetStream() != StructuredLogStream_STRUCTURED_LOG_STREAM_LOGGER {
		t.Fatalf("stream = %s, want logger", event.GetStream())
	}
	if event.GetLevel() != StructuredLogLevel_STRUCTURED_LOG_LEVEL_WARN {
		t.Fatalf("level = %s, want warn", event.GetLevel())
	}
	if event.GetFields()["attempt"] != "2" {
		t.Fatalf("attempt field = %q, want 2", event.GetFields()["attempt"])
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

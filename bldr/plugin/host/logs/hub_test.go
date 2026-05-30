package plugin_host_logs

import (
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestHubAssignsMonotonicSequencesAndBoundsRetainedHistory(t *testing.T) {
	hub := NewHub(
		WithRetainedEventLimit(3),
		WithClock(func() time.Time { return time.Unix(123, 0) }),
	)
	view := hub.OpenView(nil, nil)
	defer view.Release()

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
	view := hub.OpenView(nil, nil)
	defer view.Release()

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
	view := hub.OpenView(nil, nil)
	defer view.Release()

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

func TestHubRetainsHistoryOnlyWhileViewsAreOpen(t *testing.T) {
	hub := NewHub(WithRetainedEventLimit(10))

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

	view := hub.OpenView(nil, nil)
	secondView := hub.OpenView(nil, nil)

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

	view.Release()
	got = eventSequences(hub.Snapshot(nil, nil).GetEvents())
	if !equalSequences(got, want) {
		t.Fatalf("retained sequences with second view = %v, want %v", got, want)
	}

	secondView.Release()
	if got := len(hub.Snapshot(nil, nil).GetEvents()); got != 0 {
		t.Fatalf("retained events after last release = %d, want 0", got)
	}

	resp, err = hub.Emit(&StructuredLogEvent{PluginId: "runner"})
	if err != nil {
		t.Fatalf("Emit after last release: %v", err)
	}
	if resp.GetSequence() != 4 {
		t.Fatalf("sequence after last release = %d, want 4", resp.GetSequence())
	}
	reopenedView := hub.OpenView(nil, nil)
	defer reopenedView.Release()
	if got := len(reopenedView.Snapshot().GetEvents()); got != 0 {
		t.Fatalf("retained events after reopening = %d, want 0", got)
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

func TestViewUpdatesCoalesceAndCloseOnRelease(t *testing.T) {
	hub := NewHub()
	view := hub.OpenView(nil, nil)
	ch := view.Updates()

	view.Set(nil, nil)
	view.Set(nil, nil)

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
	hub := NewHub()
	assertHubBroadcasts := func(name string, mutate func()) {
		t.Helper()

		locked := hub.bcast.Lock()
		ch := locked.WaitCh()
		locked.Unlock()

		mutate()
		select {
		case <-ch:
		default:
			t.Fatalf("%s did not broadcast", name)
		}
	}

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
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	hub := NewHub(WithRetainedEventLimit(10))
	release := AttachHostLogrusHook(nil, log, hub)
	defer release()
	if got := len(log.Hooks[logrus.WarnLevel]); got != 1 {
		t.Fatalf("warn hooks = %d, want 1", got)
	}

	view := hub.OpenView(nil, nil)
	defer view.Release()

	log.WithFields(logrus.Fields{
		"plugin-id":    "runner",
		"instance-key": "main",
		"attempt":      2,
	}).Warn("host captured")

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

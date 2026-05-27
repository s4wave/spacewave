package spacewave_loader_ui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type recordingSender struct {
	mu        sync.Mutex
	calls     []progressCall
	dismissed int
}

type progressCall struct {
	fraction float32
	text     string
}

func (r *recordingSender) SendProgress(fraction float32, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, progressCall{fraction: fraction, text: text})
	return nil
}

func (r *recordingSender) SendDismiss() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dismissed++
	return nil
}

func (r *recordingSender) dismissCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dismissed
}

func (r *recordingSender) last() progressCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return progressCall{}
	}
	return r.calls[len(r.calls)-1]
}

func newTrackerForTest(pluginIDs []string) (*Tracker, *recordingSender) {
	sender := &recordingSender{}
	tracker := NewTracker(sender, logrus.NewEntry(logrus.New()), pluginIDs)
	return tracker, sender
}

func TestProgressTrackerInitialIndeterminate(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})
	tracker.Render()
	got := sender.last()
	if got.fraction != -1 {
		t.Fatalf("initial fraction = %v, want -1 (indeterminate)", got.fraction)
	}
	if !strings.Contains(got.text, "Preparing") {
		t.Fatalf("initial text = %q, want contains 'Preparing'", got.text)
	}
}

func TestProgressTrackerDeterminateProgress(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "spacewave-web", "web"})
	tracker.MarkRunning("spacewave-core", true)
	got := sender.last()
	if got.fraction == -1 {
		t.Fatalf("fraction still indeterminate after one resolve")
	}
	if got.fraction <= 0 || got.fraction >= 1 {
		t.Fatalf("fraction = %v, want in (0, 1)", got.fraction)
	}
	if !strings.Contains(got.text, "spacewave-web") {
		t.Fatalf("phase label = %q, want next pending (spacewave-web)", got.text)
	}
}

func TestProgressTrackerFetchErrorShowsRetryMessage(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core"})
	tracker.SetFetchStatus(&FetchStatus{
		LastErr:     "dial tcp: lookup spacewave.app: no such host",
		Attempts:    1,
		NextRetryAt: time.Now().Add(5 * time.Second),
	})
	got := sender.last()
	if got.fraction != -1 {
		t.Fatalf("retry fraction = %v, want -1", got.fraction)
	}
	if !strings.Contains(got.text, "Waiting for network") {
		t.Fatalf("retry text = %q, want contains 'Waiting for network'", got.text)
	}
	if !strings.Contains(got.text, "retry in") {
		t.Fatalf("retry text = %q, want countdown 'retry in'", got.text)
	}
}

func TestProgressTrackerConnectingMessage(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core"})
	tracker.SetFetchStatus(&FetchStatus{
		Fetching: true,
	})
	got := sender.last()
	if got.fraction != -1 {
		t.Fatalf("connecting fraction = %v, want -1", got.fraction)
	}
	if !strings.Contains(got.text, "Connecting") {
		t.Fatalf("connecting text = %q, want contains 'Connecting'", got.text)
	}
}

func TestProgressTrackerFetchSuccessFallsThroughToPlugins(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})
	tracker.SetFetchStatus(&FetchStatus{HasConfig: true})
	tracker.MarkRunning("spacewave-core", true)
	got := sender.last()
	if got.fraction <= 0 || got.fraction >= 1 {
		t.Fatalf("fraction = %v, want plugin-level determinate", got.fraction)
	}
	if !strings.Contains(got.text, "web") {
		t.Fatalf("phase label = %q, want next pending (web)", got.text)
	}
}

func TestProgressTrackerDismissesWhenAllPluginsRunning(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})
	tracker.MarkRunning("spacewave-core", true)
	if sender.dismissCount() != 0 {
		t.Fatalf("dismissed after partial progress: count=%d", sender.dismissCount())
	}
	tracker.MarkRunning("web", true)
	if sender.dismissCount() != 1 {
		t.Fatalf("dismiss count after all running = %d, want 1", sender.dismissCount())
	}
	got := sender.last()
	if got.fraction != 1 {
		t.Fatalf("final fraction = %v, want 1.0", got.fraction)
	}
	if !strings.Contains(got.text, "Ready") {
		t.Fatalf("final text = %q, want contains 'Ready'", got.text)
	}
	select {
	case <-tracker.Done():
	case <-time.After(time.Second):
		t.Fatalf("tracker.Done() did not close after full progress")
	}
	prevCalls := len(sender.calls)
	tracker.MarkRunning("web", false)
	tracker.SetFetchStatus(&FetchStatus{LastErr: "boom"})
	if len(sender.calls) != prevCalls {
		t.Fatalf("progress calls after dismiss: got %d, want %d", len(sender.calls), prevCalls)
	}
	if sender.dismissCount() != 1 {
		t.Fatalf("dismiss count after post-dismiss updates = %d", sender.dismissCount())
	}
}

func TestLoaderRetryOnNetworkFailure(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})

	tracker.SetFetchStatus(&FetchStatus{Fetching: true})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Connecting") {
		t.Fatalf("connecting phase = %+v, want indeterminate 'Connecting'", got)
	}

	tracker.SetFetchStatus(&FetchStatus{
		LastErr:     "dial tcp: no route to host",
		Attempts:    1,
		NextRetryAt: time.Now().Add(5 * time.Second),
	})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Waiting for network") {
		t.Fatalf("first failure = %+v, want 'Waiting for network' retry", got)
	}

	tracker.SetFetchStatus(&FetchStatus{
		LastErr:     "dial tcp: no route to host",
		Attempts:    2,
		NextRetryAt: time.Now().Add(10 * time.Second),
	})
	if got := sender.last(); !strings.Contains(got.text, "retry in") {
		t.Fatalf("second failure = %+v, want countdown label", got)
	}

	tracker.SetFetchStatus(&FetchStatus{HasConfig: true})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Preparing") {
		t.Fatalf("post-recovery = %+v, want 'Preparing' indeterminate", got)
	}

	tracker.MarkRunning("spacewave-core", true)
	if got := sender.last(); got.fraction <= 0 || got.fraction >= 1 {
		t.Fatalf("mid-progress = %+v, want fraction in (0, 1)", got)
	}
	tracker.MarkRunning("web", true)
	if sender.dismissCount() != 1 {
		t.Fatalf("dismiss count after full load = %d, want 1", sender.dismissCount())
	}
}

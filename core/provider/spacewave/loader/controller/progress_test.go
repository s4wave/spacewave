package spacewave_loader_controller

import (
	"strings"
	"sync"
	"testing"
	"time"

	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/sirupsen/logrus"
)

// recordingSender captures every SendProgress / SendDismiss call on a tracker
// so tests can assert the final helper state after a series of state
// transitions.
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

func newTrackerForTest(pluginIDs []string) (*progressTracker, *recordingSender) {
	sender := &recordingSender{}
	tracker := newProgressTracker(sender, logrus.NewEntry(logrus.New()), pluginIDs)
	return tracker, sender
}

func TestProgressTrackerInitialIndeterminate(t *testing.T) {
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})
	tracker.render()
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
	tracker.markRunning("spacewave-core", true)
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
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{
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
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{
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
	// Launcher has a config; plugin-level progress should win.
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{HasConfig: true})
	tracker.markRunning("spacewave-core", true)
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
	tracker.markRunning("spacewave-core", true)
	if sender.dismissCount() != 0 {
		t.Fatalf("dismissed after partial progress: count=%d", sender.dismissCount())
	}
	tracker.markRunning("web", true)
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
	// Further state changes must not emit more progress or dismiss calls.
	prevCalls := len(sender.calls)
	tracker.markRunning("web", false)
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{LastErr: "boom"})
	if len(sender.calls) != prevCalls {
		t.Fatalf("progress calls after dismiss: got %d, want %d", len(sender.calls), prevCalls)
	}
	if sender.dismissCount() != 1 {
		t.Fatalf("dismiss count after post-dismiss updates = %d, want 1", sender.dismissCount())
	}
}

func TestLoaderRetryOnNetworkFailure(t *testing.T) {
	// Simulates the launcher's DistConfig fetcher going through
	// connecting -> failure -> backoff countdown -> recovery -> plugins
	// resolve -> dismiss. The tracker renders the same UI the real loader
	// would show a user who starts offline and comes back online.
	tracker, sender := newTrackerForTest([]string{"spacewave-core", "web"})

	// 1. Fetcher kicks off (no config yet): indeterminate connecting.
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{Fetching: true})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Connecting") {
		t.Fatalf("connecting phase = %+v, want indeterminate 'Connecting'", got)
	}

	// 2. First fetch fails; next retry 5s away.
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{
		LastErr:     "dial tcp: no route to host",
		Attempts:    1,
		NextRetryAt: time.Now().Add(5 * time.Second),
	})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Waiting for network") {
		t.Fatalf("first failure = %+v, want 'Waiting for network' retry", got)
	}

	// 3. Second attempt also fails; backoff widens to ~10s.
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{
		LastErr:     "dial tcp: no route to host",
		Attempts:    2,
		NextRetryAt: time.Now().Add(10 * time.Second),
	})
	if got := sender.last(); !strings.Contains(got.text, "retry in") {
		t.Fatalf("second failure = %+v, want countdown label", got)
	}

	// 4. Network comes back: launcher publishes HasConfig=true. Tracker
	//    falls through to plugin-level progress (still indeterminate until
	//    any plugin reports Running).
	tracker.setFetchStatus(&spacewave_launcher.FetchStatus{HasConfig: true})
	if got := sender.last(); got.fraction != -1 || !strings.Contains(got.text, "Preparing") {
		t.Fatalf("post-recovery = %+v, want 'Preparing' indeterminate", got)
	}

	// 5. Plugins resolve; tracker becomes determinate, then dismisses.
	tracker.markRunning("spacewave-core", true)
	if got := sender.last(); got.fraction <= 0 || got.fraction >= 1 {
		t.Fatalf("mid-progress = %+v, want fraction in (0, 1)", got)
	}
	tracker.markRunning("web", true)
	if sender.dismissCount() != 1 {
		t.Fatalf("dismiss count after full load = %d, want 1", sender.dismissCount())
	}
	select {
	case <-tracker.Done():
	case <-time.After(time.Second):
		t.Fatalf("tracker.Done() did not close after full load")
	}
}

func TestFormatRetryMessageFallbacks(t *testing.T) {
	if msg := formatRetryMessage(time.Time{}); !strings.HasSuffix(msg, "...") {
		t.Fatalf("zero-time message = %q, want fallback '...'", msg)
	}
	soon := time.Now().Add(100 * time.Millisecond)
	if msg := formatRetryMessage(soon); !strings.Contains(msg, "retrying") {
		t.Fatalf("sub-second countdown = %q, want 'retrying'", msg)
	}
}

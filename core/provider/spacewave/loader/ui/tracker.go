package spacewave_loader_ui

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const defaultHelperBinaryName = "spacewave-helper"

const defaultAppIconPath = "../Resources/app.icns"

// Sender is the minimal loader-helper client surface consumed by Tracker.
type Sender interface {
	SendProgress(fraction float32, text string) error
	SendDismiss() error
}

// FetchStatus is the loader UI's read-only view of launcher fetch progress.
type FetchStatus struct {
	Fetching    bool
	HasConfig   bool
	LastErr     string
	Attempts    uint32
	NextRetryAt time.Time
}

// Tracker maintains running state for watched plugin ids plus the latest
// launcher fetch status, and pushes progress or retry-state messages to the
// helper whenever either input changes.
type Tracker struct {
	client    Sender
	le        *logrus.Entry
	pluginIDs []string

	mu          sync.Mutex
	running     map[string]bool
	fetchStatus *FetchStatus
	dismissed   bool
	doneCh      chan struct{}
}

// NewTracker constructs a tracker for the given plugin id order.
func NewTracker(client Sender, le *logrus.Entry, pluginIDs []string) *Tracker {
	return &Tracker{
		client:    client,
		le:        le,
		pluginIDs: pluginIDs,
		running:   make(map[string]bool, len(pluginIDs)),
		doneCh:    make(chan struct{}),
	}
}

// Done returns a channel that closes after the tracker has sent the dismiss
// signal to the helper.
func (t *Tracker) Done() <-chan struct{} {
	return t.doneCh
}

// MarkRunning flips the per-plugin running state and re-renders progress.
func (t *Tracker) MarkRunning(pluginID string, running bool) {
	t.mu.Lock()
	if t.running[pluginID] == running {
		t.mu.Unlock()
		return
	}
	t.running[pluginID] = running
	t.mu.Unlock()
	t.Render()
}

// SetFetchStatus updates the cached launcher fetch status and re-renders.
func (t *Tracker) SetFetchStatus(status *FetchStatus) {
	t.mu.Lock()
	if t.fetchStatus == status {
		t.mu.Unlock()
		return
	}
	t.fetchStatus = status
	t.mu.Unlock()
	t.Render()
}

// Render pushes the current progress snapshot to the helper.
func (t *Tracker) Render() {
	t.mu.Lock()
	if t.dismissed {
		t.mu.Unlock()
		return
	}
	total := len(t.pluginIDs)
	running := 0
	var nextPending string
	for _, id := range t.pluginIDs {
		if t.running[id] {
			running++
			continue
		}
		if nextPending == "" {
			nextPending = id
		}
	}
	status := t.fetchStatus
	if total > 0 && running == total {
		t.dismissed = true
		t.mu.Unlock()
		if err := t.client.SendProgress(1.0, "Ready"); err != nil {
			t.le.WithError(err).Debug("send final progress")
		}
		if err := t.client.SendDismiss(); err != nil {
			t.le.WithError(err).Debug("send dismiss")
		}
		close(t.doneCh)
		return
	}
	t.mu.Unlock()

	if status != nil && !status.HasConfig {
		if status.LastErr != "" {
			text := formatRetryMessage(status.NextRetryAt)
			if err := t.client.SendProgress(-1, text); err != nil {
				t.le.WithError(err).Debug("send retry status")
			}
			return
		}
		if status.Fetching {
			if err := t.client.SendProgress(-1, "Connecting to Spacewave..."); err != nil {
				t.le.WithError(err).Debug("send connecting status")
			}
			return
		}
	}

	if total == 0 {
		if err := t.client.SendProgress(-1, "Preparing Spacewave..."); err != nil {
			t.le.WithError(err).Debug("send preparing status")
		}
		return
	}
	if running == 0 {
		if err := t.client.SendProgress(-1, "Preparing Spacewave..."); err != nil {
			t.le.WithError(err).Debug("send initial progress")
		}
		return
	}
	text := "Loading Spacewave..."
	if nextPending != "" {
		text = "Loading " + nextPending + "..."
	}
	fraction := float32(running) / float32(total)
	if err := t.client.SendProgress(fraction, text); err != nil {
		t.le.WithError(err).Debug("send progress")
	}
}

// ResolveHelperPathFromDirs resolves the helper binary from candidate dirs.
func ResolveHelperPathFromDirs(baseDirs []string, overrideName, goos string) (string, bool) {
	for _, baseDir := range baseDirs {
		if baseDir == "" {
			continue
		}
		path, ok := ResolveHelperPathIn(baseDir, overrideName, goos)
		if ok {
			return path, true
		}
	}
	return "", false
}

// ResolveIconPathFromDirs resolves the app icon from candidate dirs.
func ResolveIconPathFromDirs(baseDirs []string) string {
	for _, baseDir := range baseDirs {
		if baseDir == "" {
			continue
		}
		path := filepath.Clean(filepath.Join(baseDir, defaultAppIconPath))
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ResolveHelperPathIn resolves the helper binary path within baseDir.
func ResolveHelperPathIn(baseDir, overrideName, goos string) (string, bool) {
	name := overrideName
	if name == "" {
		name = defaultHelperBinaryName
		if goos == "windows" {
			name += ".exe"
		}
	}
	path := filepath.Join(baseDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

func formatRetryMessage(nextRetryAt time.Time) string {
	const prefix = "Waiting for network"
	if nextRetryAt.IsZero() {
		return prefix + "..."
	}
	remaining := time.Until(nextRetryAt)
	if remaining < time.Second {
		return prefix + " (retrying...)"
	}
	secs := int(remaining.Round(time.Second).Seconds())
	return prefix + " (retry in " + strconv.Itoa(secs) + "s)"
}

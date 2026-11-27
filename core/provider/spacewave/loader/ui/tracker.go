package ui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// defaultHelperBinaryName is the helper executable without a platform suffix.
const defaultHelperBinaryName = "spacewave-helper"

// defaultAppIconPath locates the icon relative to the helper directory.
const defaultAppIconPath = "../Resources/app.icns"

// Sender is the minimal loader-helper client surface consumed by Tracker.
type Sender interface {
	// SendProgress updates the helper's progress display.
	SendProgress(fraction float32, text string) error
	// SendDismiss closes the helper window.
	SendDismiss() error
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

// WaitDismissed waits until the helper has been dismissed or ctx ends.
func (t *Tracker) WaitDismissed(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-t.doneCh:
	}
}

// MarkRunning flips the per-plugin running state and re-renders progress.
func (t *Tracker) MarkRunning(pluginID string, running bool) {
	// Publish only changes to the plugin's running state.
	t.mu.Lock()
	if t.running[pluginID] == running {
		t.mu.Unlock()
		return
	}
	t.running[pluginID] = running
	t.mu.Unlock()

	// Send progress after releasing the state lock.
	t.Render()
}

// SetFetchStatus updates the cached launcher fetch status and re-renders.
func (t *Tracker) SetFetchStatus(status *FetchStatus) {
	// Publish only a new fetch snapshot.
	t.mu.Lock()
	if t.fetchStatus == status {
		t.mu.Unlock()
		return
	}
	t.fetchStatus = status
	t.mu.Unlock()

	// Send progress after releasing the state lock.
	t.Render()
}

// Render pushes the current progress snapshot to the helper.
func (t *Tracker) Render() {
	// Keep a dismissed helper closed.
	t.mu.Lock()
	if t.dismissed {
		t.mu.Unlock()
		return
	}

	// Count running plugins and find the next pending plugin under the lock.
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

	// Capture fetch status and dismiss the helper once all plugins run.
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

	// Show the fetch failure or connection state before loading plugins.
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

	// Show indeterminate progress until a plugin starts.
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

	// Report the fraction loaded and the next plugin to start.
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
	// Choose the platform's helper name unless the caller supplied one.
	name := overrideName
	if name == "" {
		name = defaultHelperBinaryName
		if goos == "windows" {
			name += ".exe"
		}
	}

	// Resolve the helper only when the candidate exists.
	path := filepath.Join(baseDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// formatRetryMessage includes the retry delay when one is scheduled.
func formatRetryMessage(nextRetryAt time.Time) string {
	// Show a generic wait when no retry is scheduled.
	const prefix = "Waiting for network"
	if nextRetryAt.IsZero() {
		return prefix + "..."
	}

	// Report a due retry or its remaining delay.
	remaining := time.Until(nextRetryAt)
	if remaining < time.Second {
		return prefix + " (retrying...)"
	}
	secs := int(remaining.Round(time.Second).Seconds())
	return prefix + " (retry in " + strconv.Itoa(secs) + "s)"
}

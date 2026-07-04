//go:build !js

package memlab

import (
	"path/filepath"
	"slices"

	"github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

// SnapshotSet holds paths to a set of labeled heap snapshots.
type SnapshotSet struct {
	// Dir is the directory containing the snapshots.
	Dir string
	// Labels tracks snapshot labels in insertion order.
	Labels []string
	// Snapshots maps label to file path.
	Snapshots map[string]string
}

// SnapshotLabels returns the snapshot labels in insertion order.
// Use the Snapshots map for path lookup.
func (s *SnapshotSet) SnapshotLabels() []string {
	return slices.Clone(s.Labels)
}

// Path returns the snapshot file path for a label.
func (s *SnapshotSet) Path(label string) string {
	return s.Snapshots[label]
}

// CaptureSnapshot captures a single labeled snapshot into the set.
func (s *SnapshotSet) CaptureSnapshot(ctx playwright.BrowserContext, page playwright.Page, label string) error {
	outPath := filepath.Join(s.Dir, label+".heapsnapshot")
	path, err := CaptureHeapSnapshot(ctx, page, outPath)
	if err != nil {
		return errors.Wrapf(err, "capture snapshot %q", label)
	}
	if _, exists := s.Snapshots[label]; !exists {
		s.Labels = append(s.Labels, label)
	}
	s.Snapshots[label] = path
	return nil
}

// NewSnapshotSet creates a new SnapshotSet writing to dir.
func NewSnapshotSet(dir string) *SnapshotSet {
	return &SnapshotSet{
		Dir:       dir,
		Labels:    make([]string, 0, 3),
		Snapshots: make(map[string]string),
	}
}

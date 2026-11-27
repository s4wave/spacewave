//go:build !js

package memlab

import (
	"path/filepath"
	"strings"

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

// CaptureSnapshot captures a single labeled snapshot into the set.
func (s *SnapshotSet) CaptureSnapshot(ctx playwright.BrowserContext, page playwright.Page, label string) error {
	// Capture the heap before recording its label and path.
	outPath := filepath.Join(s.Dir, label+".heapsnapshot")
	path, err := CaptureHeapSnapshot(ctx, page, outPath)
	if err != nil {
		return errors.Wrapf(err, "capture snapshot %q", label)
	}

	// Preserve first-capture order when replacing an existing label.
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

// BuildSnapshotArg builds the ordered label=path argument for analyze.js.
func (s *SnapshotSet) BuildSnapshotArg() (string, error) {
	// Encode the captured paths in label order.
	var pairs []string
	for _, label := range s.Labels {
		path, ok := s.Snapshots[label]
		if !ok {
			return "", errors.Errorf("missing snapshot path for label %q", label)
		}
		pairs = append(pairs, label+"="+path)
	}
	if len(pairs) == 0 {
		return "", errors.New("no snapshots to analyze")
	}
	return strings.Join(pairs, ","), nil
}

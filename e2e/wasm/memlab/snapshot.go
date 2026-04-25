//go:build !js

package memlab

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/playwright-community/playwright-go"
)

// CaptureHeapSnapshot captures a Chrome V8 heap snapshot via CDP
// HeapProfiler on the given Playwright page. It forces GC first,
// then triggers a heap snapshot, collects all chunks, and writes
// them to outPath. Returns the written file path.
func CaptureHeapSnapshot(ctx playwright.BrowserContext, page playwright.Page, outPath string) (string, error) {
	session, err := ctx.NewCDPSession(page)
	if err != nil {
		return "", errors.Wrap(err, "new CDP session")
	}
	defer session.Detach()

	// Force garbage collection before snapshot.
	if _, err := session.Send("HeapProfiler.collectGarbage", nil); err != nil {
		return "", errors.Wrap(err, "collect garbage")
	}

	// Collect snapshot chunks.
	var mu sync.Mutex
	var chunks []string

	handler := func(params any) {
		m, ok := params.(map[string]any)
		if !ok {
			return
		}
		chunk, ok := m["chunk"].(string)
		if !ok {
			return
		}
		mu.Lock()
		chunks = append(chunks, chunk)
		mu.Unlock()
	}
	session.On("HeapProfiler.addHeapSnapshotChunk", handler)

	// Trigger the snapshot (blocks until complete).
	if _, err := session.Send("HeapProfiler.takeHeapSnapshot", map[string]any{
		"reportProgress":      false,
		"captureNumericValue": true,
	}); err != nil {
		return "", errors.Wrap(err, "take heap snapshot")
	}

	session.RemoveListener("HeapProfiler.addHeapSnapshotChunk", handler)

	// Write concatenated chunks to file.
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", errors.Wrap(err, "create snapshot dir")
	}

	var sb strings.Builder
	mu.Lock()
	for _, c := range chunks {
		sb.WriteString(c)
	}
	mu.Unlock()

	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return "", errors.Wrap(err, "write snapshot")
	}

	return outPath, nil
}

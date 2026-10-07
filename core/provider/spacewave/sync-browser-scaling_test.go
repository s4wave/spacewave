//go:build goscript

package provider_spacewave

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"syscall/js"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile/writer"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	"github.com/sirupsen/logrus"
)

// syncBrowserTransport serves production push requests from a local push
// server without a network or a paid service.
type syncBrowserTransport struct {
	push *testPushServer
}

// RoundTrip serves req from the push server.
func (s *syncBrowserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	s.push.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// TestSyncBrowserDrainScaling1000 measures the small offline backlog.
func TestSyncBrowserDrainScaling1000(t *testing.T) {
	testSyncBrowserDrainScaling(t, 1000)
}

// TestSyncBrowserDrainScaling10000 measures a backlog spanning metadata pages.
func TestSyncBrowserDrainScaling10000(t *testing.T) {
	testSyncBrowserDrainScaling(t, 10000)
}

// TestSyncBrowserDrainScaling100000 measures the large offline backlog.
func TestSyncBrowserDrainScaling100000(t *testing.T) {
	testSyncBrowserDrainScaling(t, 100000)
}

// testSyncBrowserDrainScaling exercises real browser volume metadata, queue,
// and packing.
func testSyncBrowserDrainScaling(t *testing.T, count int) {
	// Skip without browser storage: the Bun runtime has none, so run with goscript test --browser.
	t.Helper()
	if nav := js.Global().Get("navigator"); nav.IsUndefined() || nav.Get("storage").IsUndefined() {
		t.Skip("browser storage is unavailable in this runtime")
	}

	// Open a fresh browser volume, deleted when the test ends, and a measured
	// pack catalog over its metadata store.
	ctx := t.Context()
	name := "pending-upload-scaling-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	volume, err := volume_browser.NewVolume(ctx, nil, &volume_browser.Config{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := volume.Delete(); err != nil {
			t.Error(err)
		}
	})
	metadata := &syncMeasuredStore{store: volume.GetKvtxStore()}
	catalog, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}

	// Build a syncer whose client pushes to a local push server.
	push := newTestPushServer(t, "https://local.invalid")
	transport := &syncBrowserTransport{push: push}
	key, peer := generateTestKeypair(t)
	client := NewSessionClient(&http.Client{Transport: transport}, "https://local.invalid", DefaultSigningEnvPrefix, key, peer.String())
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	syncer := &syncController{
		le: logrus.NewEntry(logger), store: metadata, client: client, resourceID: "browser-scaling",
		mfst: catalog, lower: push.lower(t), upper: newSyncTestBlockStore(),
	}

	// Mark count small blocks dirty.
	marks := make([]block_store_writeback.Mark, 0, count)
	for n := range count {
		data := []byte("offline-block-" + strconv.Itoa(n))
		ref, _, err := syncer.upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(data))})
	}
	markStarted := time.Now()
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}
	marked := time.Since(markStarted)

	// Drain the queue, counting metadata visits.
	metadata.visits.Store(0)
	started := time.Now()
	if err := syncer.FlushNowUnordered(ctx); err != nil {
		t.Fatal(err)
	}

	// The drain visited each mark twice and pushed one pack per page.
	if metadata.visits.Load() != int64(count*2) || metadata.page.Load() > syncDirtyPageLimit {
		t.Fatalf("unbounded queue work: visits=%d page=%d", metadata.visits.Load(), metadata.page.Load())
	}
	uploads, uploaded := push.uploadTotals()
	if uploads != (count+4095)/4096 {
		t.Fatalf("paging changed pack count: %d", uploads)
	}
	t.Logf("backend=opfs blocks=%d pack-bytes=%d pack-blocks=%d mark=%s visits=%d max-read=%d uploads=%d upload-bytes=%d drain=%s", count, syncFlushMaxPackBytes, writer.DefaultMaxBlocksPerPack, marked, metadata.visits.Load(), metadata.page.Load(), uploads, uploaded, time.Since(started))
}

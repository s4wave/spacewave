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
	"github.com/s4wave/spacewave/db/opfs"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
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

// testSyncBrowserDrainScaling exercises real OPFS metadata, queue, and packing.
func testSyncBrowserDrainScaling(t *testing.T, count int) {
	// Skip without OPFS: the Bun runtime has none, so run with goscript test --browser.
	t.Helper()
	if nav := js.Global().Get("navigator"); nav.IsUndefined() || nav.Get("storage").IsUndefined() {
		t.Skip("browser OPFS is unavailable in this runtime")
	}

	// Create a fresh OPFS directory removed when the test ends.
	ctx := t.Context()
	driver := opfs.BrowserDriver{}
	root, err := driver.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	name := "pending-upload-scaling-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	dir, err := driver.GetDirectory(root, name, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { driver.DeleteEntry(root, name, true) })

	// Open the volume and a measured pack catalog over its metadata store.
	backend := engine.NewBrowserBackend(driver, dir, name)
	volume, err := engine.Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { volume.Close() })
	metadata := &syncMeasuredStore{store: volume.MetadataStore()}
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
		mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil), upper: newSyncTestBlockStore(),
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
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}

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
	t.Logf("blocks=%d visits=%d max-read=%d uploads=%d upload-bytes=%d elapsed=%s", count, metadata.visits.Load(), metadata.page.Load(), uploads, uploaded, time.Since(started))
}

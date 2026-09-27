//go:build goscript

package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/opfs"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/volume/js/opfs/engine"
	"github.com/sirupsen/logrus"
)

// syncBrowserTransport consumes production upload requests without a paid service.
type syncBrowserTransport struct {
	// requests counts uploads made by SessionClient.
	requests int
	// bytes counts encoded HTTP request bodies.
	bytes int64
}

// RoundTrip consumes an upload and returns the successful local fixture response.
func (s *syncBrowserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	n, err := io.Copy(io.Discard, req.Body)
	if err != nil {
		return nil, err
	}
	s.requests++
	s.bytes += n
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
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
	t.Helper()
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
	transport := &syncBrowserTransport{}
	key, peer := generateTestKeypair(t)
	client := NewSessionClient(&http.Client{Transport: transport}, "https://local.invalid", DefaultSigningEnvPrefix, key, peer.String())
	client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
		return submit("local-test-ticket")
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	syncer := &syncController{
		le: logrus.NewEntry(logger), store: metadata, client: client, resourceID: "browser-scaling",
		mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil), upper: newSyncTestBlockStore(),
	}
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
	metadata.visits.Store(0)
	started := time.Now()
	if err := syncer.FlushNowUnordered(ctx); err != nil {
		t.Fatal(err)
	}
	if metadata.visits.Load() != int64(count*2) || metadata.page.Load() > syncDirtyPageLimit {
		t.Fatalf("unbounded queue work: visits=%d page=%d", metadata.visits.Load(), metadata.page.Load())
	}
	if transport.requests != (count+4095)/4096 {
		t.Fatalf("paging changed pack count: %d", transport.requests)
	}
	t.Logf("blocks=%d visits=%d max-read=%d uploads=%d upload-bytes=%d elapsed=%s", count, metadata.visits.Load(), metadata.page.Load(), transport.requests, transport.bytes, time.Since(started))
}

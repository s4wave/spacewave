//go:build !js && !wasip1

package provider_spacewave

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// TestSyncMixedUploadCost measures the production upload boundary with byte-
// limited packs and duplicate marks, using only local HTTP and storage.
func TestSyncMixedUploadCost(t *testing.T) {
	ctx := t.Context()
	metadata := newSyncTestKvStore()
	catalog, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var requests, uploaded atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			t.Error(err)
		}
		requests.Add(1)
		uploaded.Add(n)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	key, peer := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peer.String())
	client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
		return submit("local-test-ticket")
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	syncer := &syncController{
		le: logrus.NewEntry(logger), store: metadata, client: client, resourceID: "mixed-cost",
		mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil), upper: newSyncTestBlockStore(),
	}
	const count = 10000
	marks := make([]block_store_writeback.Mark, 0, count)
	var payloadBytes int64
	for n := range count {
		data := bytes.Repeat([]byte{byte(n)}, 1024+(n*7919)%32768)
		copy(data, "unique-block-"+strconv.Itoa(n))
		ref, _, err := syncer.upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(data))})
		payloadBytes += int64(len(data))
	}
	for range 2 {
		if err := syncer.MarkDirty(ctx, marks); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncer.FlushNowUnordered(ctx); err != nil {
		t.Fatal(err)
	}
	if _, pending, _ := syncer.pendingSnapshot(); pending != 0 {
		t.Fatalf("pending bytes after drain: %d", pending)
	}
	t.Logf("blocks=%d duplicate-marks=%d payload-bytes=%d uploads=%d upload-bytes=%d", count, count, payloadBytes, requests.Load(), uploaded.Load())
}

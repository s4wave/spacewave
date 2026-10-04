//go:build !js && !wasip1

package provider_spacewave

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/sirupsen/logrus"
)

// TestSyncMixedUploadCost measures the production upload boundary with byte-
// limited packs and duplicate marks, using only local HTTP and storage.
func TestSyncMixedUploadCost(t *testing.T) {
	// Open a pack catalog over in-memory metadata.
	ctx := t.Context()
	metadata := newSyncTestKvStore()
	catalog, err := manifest.New(ctx, metadata)
	if err != nil {
		t.Fatal(err)
	}

	// Build a syncer whose client pushes to a local push server.
	push := startTestPushServer(t)
	key, peer := generateTestKeypair(t)
	client := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, key, peer.String())
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	syncer := &syncController{
		le: logrus.NewEntry(logger), store: metadata, client: client, resourceID: "mixed-cost",
		mfst: catalog, lower: push.lower(t), upper: newSyncTestBlockStore(),
	}

	// Store count blocks of mixed sizes.
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

	// Mark every block dirty twice and drain the queue.
	for range 2 {
		if err := syncer.MarkDirty(ctx, marks); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncer.FlushNowUnordered(ctx); err != nil {
		t.Fatal(err)
	}

	// The drain left nothing pending; report the upload cost.
	if _, pending, _ := syncer.pendingSnapshot(); pending != 0 {
		t.Fatalf("pending bytes after drain: %d", pending)
	}
	uploads, uploaded := push.uploadTotals()
	t.Logf("blocks=%d duplicate-marks=%d payload-bytes=%d uploads=%d upload-bytes=%d", count, count, payloadBytes, uploads, uploaded)
}

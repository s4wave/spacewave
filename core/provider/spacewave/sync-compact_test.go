package provider_spacewave

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// compactTestCloud serves pack bytes to the lower store and records pushes.
type compactTestCloud struct {
	mtx   sync.Mutex
	packs map[string][]byte
	// pushes are the X-Replaces-Pack-IDs values of each push, in order.
	pushes []string
	// pulls counts sync pull requests.
	pulls int
	// reject, when set, answers pushes with this status and error body.
	reject     int
	rejectBody string
	// pushed, when set, receives one value per accepted push.
	pushed chan struct{}
}

// ServeHTTP accepts pushes into packs and answers pulls with an empty delta.
func (c *compactTestCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if strings.HasSuffix(r.URL.Path, "/sync/pull") {
		c.pulls++
		w.WriteHeader(http.StatusOK)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c.pushes = append(c.pushes, r.Header.Get("X-Replaces-Pack-IDs"))
	if c.reject != 0 {
		w.WriteHeader(c.reject)
		_, _ = w.Write([]byte(c.rejectBody))
		return
	}
	c.packs[r.Header.Get("X-Pack-ID")] = body
	w.WriteHeader(http.StatusOK)
	if c.pushed != nil {
		c.pushed <- struct{}{}
	}
}

// open returns a reader over a stored pack.
func (c *compactTestCloud) open(packID string, size int64) (*packfile_store.PackReader, error) {
	c.mtx.Lock()
	data := c.packs[packID]
	c.mtx.Unlock()
	return packfile_store.NewPackReader(packID, size, &syncPackTransport{data: data}), nil
}

// newCompactTestController commits one small pack per block list, in order,
// and returns a controller whose lower store reads them from cloud.
func newCompactTestController(t *testing.T, cloud *compactTestCloud, packs [][]string) *syncController {
	t.Helper()
	ctx := t.Context()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	entries := make([]*packfile.PackfileEntry, len(packs))
	for i, datas := range packs {
		var buf bytes.Buffer
		idx := 0
		result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
			if idx >= len(datas) {
				return nil, nil, nil
			}
			data := []byte(datas[idx])
			idx++
			h, err := hash.Sum(hash.RecommendedHashType, data)
			return h, &block.StoredBlock{Data: data, RefsKnown: true}, err
		})
		if err != nil {
			t.Fatalf("pack %d: %v", i, err)
		}
		id := "pack-" + strconv.Itoa(i)
		cloud.packs[id] = buf.Bytes()
		entries[i] = &packfile.PackfileEntry{
			Id:                 id,
			BloomFilter:        result.BloomFilter,
			BloomFormatVersion: packfile.BloomFormatVersionV1,
			BlockCount:         result.BlockCount,
			SizeBytes:          result.BytesWritten,
			Sequence:           uint64(i + 1), //nolint:gosec // small test index.
		}
	}
	if err := mfst.ApplyDelta(ctx, entries, nil); err != nil {
		t.Fatalf("apply entries: %v", err)
	}

	srv := httptest.NewServer(cloud)
	t.Cleanup(srv.Close)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())
	cli.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, fn func(string) error) error {
		return fn("ticket-push")
	}

	lower := packfile_store.NewPackfileStore(cloud.open, nil)
	lower.UpdateManifest(mfst.GetEntries())
	return &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      newSyncTestKvStore(),
		client:     cli,
		resourceID: "test-res",
		mfst:       mfst,
		lower:      lower,
		upper:      newSyncTestBlockStore(),
	}
}

// compactTestPacks returns n single-block packs plus one block repeated in the
// first two packs.
func compactTestPacks(n int) [][]string {
	packs := make([][]string, n)
	for i := range packs {
		packs[i] = []string{"block-" + strconv.Itoa(i)}
	}
	packs[0] = append(packs[0], "shared")
	packs[1] = append(packs[1], "shared")
	return packs
}

// TestSyncControllerCompactMergesSmallPacks verifies a merge replaces every
// small pack with one pack that serves every input block.
func TestSyncControllerCompactMergesSmallPacks(t *testing.T) {
	ctx := t.Context()
	cloud := &compactTestCloud{packs: make(map[string][]byte)}
	packs := compactTestPacks(compactMinPacks + 1)
	s := newCompactTestController(t, cloud, packs)

	if err := s.CompactNow(ctx); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(cloud.pushes) != 1 {
		t.Fatalf("pushes = %d, want 1", len(cloud.pushes))
	}
	var want []string
	for i := range packs {
		want = append(want, "pack-"+strconv.Itoa(i))
	}
	if got := cloud.pushes[0]; got != strings.Join(want, ",") {
		t.Fatalf("replaced = %q, want inputs oldest first", got)
	}

	entries := s.mfst.GetEntries()
	if len(entries) != 1 || entries[0].GetBlockCount() != uint64(len(packs)+1) { //nolint:gosec // small test count.
		t.Fatalf("manifest after merge = %v, want one pack of %d blocks", entries, len(packs)+1)
	}
	for _, datas := range packs {
		for _, data := range datas {
			h, err := hash.Sum(hash.RecommendedHashType, []byte(data))
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := s.lower.GetBlock(ctx, block.NewBlockRef(h))
			if err != nil || string(got) != data {
				t.Fatalf("read %q after merge = %q, %v", data, got, err)
			}
		}
	}

	// The merged pack is not small enough to merge again with nothing else.
	if err := s.CompactNow(ctx); err != nil || len(cloud.pushes) != 1 {
		t.Fatalf("second compact pushed again: pushes=%d err=%v", len(cloud.pushes), err)
	}
}

// TestSyncControllerCompactAfterOutsideFlush verifies the scheduler merges
// after a flush it did not run, such as a caller waiting on its operation.
func TestSyncControllerCompactAfterOutsideFlush(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cloud := &compactTestCloud{packs: make(map[string][]byte), pushed: make(chan struct{}, 1)}
	s := newCompactTestController(t, cloud, compactTestPacks(compactMinPacks+1))
	s.conf = &SyncConfig{}

	done := make(chan error, 1)
	go func() { done <- s.Execute(ctx) }()
	if err := s.FlushNowUnordered(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	select {
	case <-cloud.pushed:
	case <-ctx.Done():
		t.Fatal("no merge after an outside flush drained the queue")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("execute: %v", err)
	}
}

// TestSyncControllerCompactWaitsForMinimum verifies fewer than compactMinPacks
// small packs are left alone.
func TestSyncControllerCompactWaitsForMinimum(t *testing.T) {
	cloud := &compactTestCloud{packs: make(map[string][]byte)}
	s := newCompactTestController(t, cloud, compactTestPacks(compactMinPacks-1))
	if err := s.CompactNow(t.Context()); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(cloud.pushes) != 0 {
		t.Fatalf("pushes = %d, want none below the minimum", len(cloud.pushes))
	}
}

// TestSyncControllerCompactConflictPulls verifies a replacement conflict fails
// once without retry and pulls before planning again.
func TestSyncControllerCompactConflictPulls(t *testing.T) {
	ctx := t.Context()
	cloud := &compactTestCloud{
		packs:      make(map[string][]byte),
		reject:     http.StatusConflict,
		rejectBody: `{"code":"pack_replacement_conflict","message":"conflict"}`,
	}
	s := newCompactTestController(t, cloud, compactTestPacks(compactMinPacks))

	if err := s.CompactNow(ctx); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(cloud.pushes) != 1 || cloud.pulls != 1 {
		t.Fatalf("pushes=%d pulls=%d, want one push and one pull", len(cloud.pushes), cloud.pulls)
	}
	if got := len(s.mfst.GetEntries()); got != compactMinPacks {
		t.Fatalf("manifest entries = %d, want the %d unmerged packs", got, compactMinPacks)
	}
}

// TestSyncControllerCompactFailureWaitsForPull verifies a failed merge is not
// planned again until a pull refreshes the manifest.
func TestSyncControllerCompactFailureWaitsForPull(t *testing.T) {
	ctx := t.Context()
	cloud := &compactTestCloud{
		packs:      make(map[string][]byte),
		reject:     http.StatusBadRequest,
		rejectBody: `{"code":"invalid_header","message":"bad"}`,
	}
	s := newCompactTestController(t, cloud, compactTestPacks(compactMinPacks))

	if err := s.CompactNow(ctx); err == nil {
		t.Fatal("expected the rejected merge to fail")
	}
	if err := s.CompactNow(ctx); err != nil || len(cloud.pushes) != 1 {
		t.Fatalf("held merge pushed again: pushes=%d err=%v", len(cloud.pushes), err)
	}

	cloud.reject = 0
	if err := s.PullNow(ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if err := s.CompactNow(ctx); err != nil {
		t.Fatalf("compact after pull: %v", err)
	}
	if len(cloud.pushes) != 2 {
		t.Fatalf("pushes = %d, want a new merge after the pull", len(cloud.pushes))
	}
}

// TestCheckPackKeysRejectsMissingBlock verifies the key-set check catches a
// merged pack that dropped an input block.
func TestCheckPackKeysRejectsMissingBlock(t *testing.T) {
	var buf bytes.Buffer
	if err := kvfile.Write(&buf, [][]byte{[]byte("a")}, func(io.Writer, []byte) (uint64, error) { return 0, nil }); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{}{"a": {}, "b": {}}
	if err := checkPackKeys(buf.Bytes(), want); err == nil {
		t.Fatal("expected a missing key to fail the check")
	}
}

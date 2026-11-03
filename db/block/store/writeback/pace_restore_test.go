package block_store_writeback

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/store/writeback/pace"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// TestPacedRestoreKeepsForegroundAvailable restores independent store copies,
// compares their foreground writes, and verifies remote-copy eligibility.
func TestPacedRestoreKeepsForegroundAvailable(t *testing.T) {
	// Build a reproducible uploaded store whose markers can be restored.
	ctx := t.Context()
	source := newTestEnv(t)
	original := source.newStore(t, true)
	remote := newInmemBlockStore()
	var refs []*block.BlockRef
	for i := range 32 {
		ref, _, err := original.PutBlock(ctx, fmt.Appendf(nil, "restored-%d", i), leafOpts)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	upload, stopUpload := context.WithCancel(ctx)
	uploaded := make(chan error, 1)
	go func() { uploaded <- original.Upload(upload, remote) }()
	for {
		status, changed := original.GetStatus()
		if status.Pending == 0 {
			break
		}
		<-changed
	}
	stopUpload()
	<-uploaded

	// Stream one consistent snapshot into each independent restored store.
	restore := func() *Store {
		t.Helper()
		copied := store_kvtx_inmem.NewStore()
		reader, err := source.kv.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Discard()
		writer, err := copied.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Discard()
		if err := reader.ScanPrefix(ctx, nil, func(key, value []byte) error {
			return writer.Set(ctx, append([]byte(nil), key...), append([]byte(nil), value...))
		}); err != nil {
			t.Fatal(err)
		}
		if err := writer.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		env := &testEnv{kv: copied}
		env.open(t)
		return env.newStore(t, true)
	}
	baseline, paced := restore(), restore()
	before := time.Now()
	if _, _, err := baseline.PutBlock(ctx, []byte("foreground"), leafOpts); err != nil {
		t.Fatal(err)
	}
	baselineLatency := time.Since(before)

	// Hold actual eviction inside the policy while a foreground write completes.
	policy := pace.New(logrus.NewEntry(logrus.New()), nil, pace.Config{Paused: true})
	started := make(chan struct{})
	eviction, cancel := context.WithCancel(ctx)
	ended := make(chan error, 1)
	go func() {
		_, err := paced.Evict(eviction, remote, 1<<30, nil, func(ctx context.Context, n int64) error { close(started); return policy.Admit(ctx, n) })
		ended <- err
	}()
	<-started
	foreground, stopForeground := context.WithTimeout(ctx, time.Second)
	before = time.Now()
	protected, _, err := paced.PutBlock(foreground, []byte("foreground"), leafOpts)
	pacedLatency := time.Since(before)
	stopForeground()
	cancel()
	<-ended
	if err != nil {
		t.Fatal("foreground write waited behind paused eviction", err)
	}
	t.Logf("restored foreground baseline=%v paced=%v", baselineLatency, pacedLatency)

	// Resume with the zero policy: only proven remote copies may leave locally.
	unpaced := pace.New(logrus.NewEntry(logrus.New()), nil, pace.Config{})
	result, err := paced.Evict(ctx, remote, 1<<30, nil, unpaced.Admit)
	if err != nil || result.Evicted != len(refs) {
		t.Fatal(result, err)
	}
	if exists, err := paced.GetBlockExists(ctx, protected); err != nil || !exists {
		t.Fatal("unuploaded foreground data was evicted", exists, err)
	}
	for _, ref := range refs {
		if _, err := remote.GetStoredBlock(ctx, ref); err != nil {
			t.Fatal("durable remote copy disappeared", err)
		}
	}
}

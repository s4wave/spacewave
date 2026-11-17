package packsweep

import (
	"context"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestPlacementKilled places a pack's keys over more than one transaction,
// killing the placement after its first: the keys the first placed do not
// count while the pack is unadopted, so a removal keeps the packs that hold
// them. A placement run again adopts the pack and places every key, and a
// hold on the pack places none.
func TestPlacementKilled(t *testing.T) {
	// Build more keys than one transaction places.
	ctx := context.Background()
	keys := make([][]byte, placeBatch+1)
	for i := range keys {
		ref, err := block.BuildBlockRef([]byte("block "+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = packfile.BlockKey(ref.GetHash())
	}

	// Kill the placement after its first transaction.
	store := &failCommits{Store: store_kvtx_inmem.NewStore(), after: 1}
	p := &compactPack{id: "new", victims: []string{"old"}}
	if err := writeCompactPack(ctx, store, p); err != nil {
		t.Fatal(err)
	}
	store.commits = 0
	if err := placePack(ctx, store, p, keys); !errors.Is(err, errKilled) {
		t.Fatalf("placement = %v, want killed", err)
	}
	checkPlaced(t, store, keys, false)

	// Place again, then hold the pack.
	store.after = -1
	p.adopted = false
	if err := placePack(ctx, store, p, keys); err != nil {
		t.Fatal(err)
	}
	checkPlaced(t, store, keys, true)
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(packRecordPrefix+p.id), nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	checkPlaced(t, store, keys, false)
}

// checkPlaced checks whether the first and the last key are placed.
func checkPlaced(t *testing.T, store kvtx.Store, keys [][]byte, want bool) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	placement := newPlacement(tx)
	for _, key := range [][]byte{keys[0], keys[len(keys)-1]} {
		if got, err := placement.placed(ctx, Tail(key)); err != nil || got != want {
			t.Fatalf("placed = %v, %v; want %v", got, err, want)
		}
	}
}

// errKilled is the error of a killed commit.
var errKilled = errors.New("killed")

// failCommits fails every commit after the first after, counting from when
// commits was last zeroed; a negative after fails none.
type failCommits struct {
	kvtx.Store
	after, commits int
}

// NewTransaction opens a transaction whose commit may fail.
func (f *failCommits) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := f.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &failCommitTx{Tx: tx, f: f}, nil
}

// failCommitTx is a transaction of failCommits.
type failCommitTx struct {
	kvtx.Tx
	f *failCommits
}

// Commit fails or commits.
func (t *failCommitTx) Commit(ctx context.Context) error {
	if t.f.after >= 0 && t.f.commits >= t.f.after {
		return errKilled
	}
	t.f.commits++
	return t.Tx.Commit(ctx)
}

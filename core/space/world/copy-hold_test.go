package space_world

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
)

// TestCopyHoldsWhatItSkips walks the fence and backfill copy through a Volume
// that frees once a record is grace old. Each case selects blocks stored with
// no holder whose downs are past grace, which the next collection nets to zero
// and frees unless the copy holds them: a cache fill the copy is asked for as
// a root, a proved block a collection runs beside between the copy's check and
// the Land of its parent, and a stored parent with no proof over a proved
// child. A collection right after the copy, and another once the copy's own
// records aged, keep every block the copy holds. No run-through reaches these:
// the cases need a grace that ages the downs before the copy, and blocks
// nothing holds under a copied root.
func TestCopyHoldsWhatItSkips(t *testing.T) {
	// Build an adapter over a Volume with a grace long enough to step through.
	ctx := t.Context()
	const grace = 300 * time.Millisecond
	a, collect := newVolumeAdapter(t, 1<<20, grace)
	store := a.pages.StoreOps

	// build returns the entry of a block referencing children.
	build := func(name string, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}

	// put stores a block that nothing holds, as a cache fill is stored.
	put := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		e := build(name, children...)
		putUnheld(t, store, e)
		return e.Ref
	}

	// keep keeps a page in a's pages, as a writer keeps its pages.
	keep := func(name string, children ...*block.BlockRef) *block.BlockRef {
		t.Helper()
		e := build(name, children...)
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{e}); err != nil {
			t.Fatal(err)
		}
		return e.Ref
	}

	// stored reports whether the Volume stores ref.
	stored := func(ref *block.BlockRef) bool {
		t.Helper()
		_, found, err := store.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	// copyCase stores a case's blocks with no holder and lets their downs
	// age past grace, so the next collection nets them to zero and frees
	// them in the same pass, and copies its root under its own fence root
	// while such a collection may run. A collection right after the copy
	// keeps every block the copy holds, since the stage the copy held them
	// under stops every free. One after the copy's records aged frees a
	// loose block nothing references and keeps the held blocks, since the
	// fence root holds them.
	copyCase := func(name string, build func() (root *block.BlockRef, held []*block.BlockRef, visited func(*block.BlockRef, []byte))) {
		t.Helper()
		// Store the blocks, and a loose block nothing references.
		root, held, visited := build()
		loose := put(name + " loose")

		// Age their downs past grace: a down nets once grace old, and a
		// block it takes to zero is ripe at once, so the next collection
		// frees each one the copy does not hold first.
		time.Sleep(grace)

		// Copy the root and hold it.
		opts := block.GraphCopyOptions{Forget: true, Visited: visited}
		if err := a.copyAndHold(ctx, fenceRootPrefix+name, []*block.BlockRef{root}, opts, false); err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}

		// Collect right after the copy and once it aged.
		for i, step := range []string{"right after the copy", "once the copy aged"} {
			if i != 0 {
				time.Sleep(grace)
			}
			collect()
			for _, ref := range held {
				if !stored(ref) {
					t.Errorf("%s: %s: the collection freed held block %s", name, step, ref.MarshalString())
				}
			}
		}
		if stored(loose) {
			t.Errorf("%s: the collection kept the loose block, so it checked no free", name)
		}
	}

	// A cache fill the copy is asked for as a root.
	copyCase("fill", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		fill := put("fill")
		return fill, []*block.BlockRef{fill}, nil
	})

	// A proved block a kept page references beside a kept leaf, with a
	// collection between the copy's check of the page's children and the
	// page's Land.
	copyCase("page", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		proved := put("proved")
		if err := block.MarkRootsComplete(ctx, store, []*block.BlockRef{proved}); err != nil {
			t.Fatal(err)
		}
		leaf := keep("leaf")
		page := keep("page", proved, leaf)
		return page, []*block.BlockRef{page, proved, leaf}, func(ref *block.BlockRef, _ []byte) {
			if ref.EqualsRef(leaf) {
				collect()
			}
		}
	})

	// A stored parent with no proof over a proved child.
	copyCase("parent", func() (*block.BlockRef, []*block.BlockRef, func(*block.BlockRef, []byte)) {
		child := put("child")
		parent := put("parent", child)
		if err := block.MarkRootsComplete(ctx, store, []*block.BlockRef{child}); err != nil {
			t.Fatal(err)
		}
		return parent, []*block.BlockRef{parent, child}, nil
	})
}

// TestCopyCountsLinkedFiles runs the fence copy over a Volume whose copy path
// reads a linked folder after the Volume, as the local provider's does, and
// tries to make it lose a block or serve wrong bytes. A parent references
// a chunk the Volume and a linked file both hold, and a chunk only a linked
// file holds. The copy stores the first under the fence root, stores nothing
// for the second, and proves the parent. Then both files change under a
// restored mtime, so only the hash check notices: collections past grace keep
// the Volume's chunk with its bytes, the file-only chunk reads as missing and
// never as the file's new bytes, and a copy that reads every block again fails
// on the file-only chunk instead of storing wrong bytes.
func TestCopyCountsLinkedFiles(t *testing.T) {
	// Build an adapter whose kept pages read the Volume, then the files.
	ctx := t.Context()
	const grace = 300 * time.Millisecond
	a, collect := newVolumeAdapter(t, 1<<20, grace)
	store := a.pages.StoreOps
	xfrm := block_transform.NewTransformerWithSteps(nil)
	files := block_store_filesegment.NewStore(block_store_filesegment.NewIndex(store_kvtx_inmem.NewStore(), nil))
	files.SetTransformer(xfrm)
	a.pages = block.NewPendingStore(&linkedBucket{
		StoreOps:    store,
		LeasedRoots: store.(block.LeasedRoots),
		RootProofs:  store.(block.RootProofs),
		files:       files,
	}, 1<<20)

	// link writes a one-chunk file and links it, returning its path, bytes
	// and chunk ref.
	dir := t.TempDir()
	link := func(name string) (string, []byte, *block.BlockRef) {
		t.Helper()
		path := filepath.Join(dir, name)
		data := bytes.Repeat([]byte(name), 4096)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := block_store_filesegment.LinkFile(ctx, files.GetIndex(), path, nil, xfrm, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		chunks := b.GetChunkIndex().GetChunks()
		if len(chunks) != 1 {
			t.Fatalf("%s: %d chunks, want 1", name, len(chunks))
		}
		return path, data, chunks[0].GetDataRef()
	}
	sharedPath, sharedData, shared := link("shared")
	onlyPath, _, only := link("only")

	// The Volume holds the shared chunk and the parent, with no holder.
	parentData := append([]byte("parent"), shared.MarshalString()+only.MarshalString()...)
	parent, err := block.BuildBlockRef(parentData, nil)
	if err != nil {
		t.Fatal(err)
	}
	putUnheld(t, store,
		&block.PutBatchEntry{Ref: shared, Data: sharedData, RefsKnown: true},
		&block.PutBatchEntry{Ref: parent, Data: parentData, Refs: []*block.BlockRef{shared, only}, RefsKnown: true},
	)

	// stored reports whether the Volume stores ref, failing on wrong bytes.
	stored := func(ref *block.BlockRef, want []byte) bool {
		t.Helper()
		data, found, err := store.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		if found && want != nil && !bytes.Equal(data, want) {
			t.Fatalf("the Volume serves wrong bytes for %s", ref.MarshalString())
		}
		return found
	}

	// The fence copy holds the shared chunk, stores nothing for the
	// file-only one, and proves the parent.
	if err := a.copyAndHold(ctx, fenceRootPrefix+"linked", []*block.BlockRef{parent}, block.GraphCopyOptions{Forget: true}, false); err != nil {
		t.Fatal(err)
	}
	if !stored(shared, sharedData) || !stored(parent, parentData) {
		t.Fatal("the copy did not hold the Volume's blocks")
	}
	if stored(only, nil) {
		t.Fatal("the copy stored the file-only chunk in the Volume")
	}
	proved, err := block.RootsComplete(ctx, store, []*block.BlockRef{parent})
	if err != nil {
		t.Fatal(err)
	}
	if !proved[0] {
		t.Fatal("the copy did not prove the parent")
	}

	// Change both files under their own size and mtime.
	for _, path := range []string{sharedPath, onlyPath} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[10] ^= 0xff
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, time.Now(), fi.ModTime()); err != nil {
			t.Fatal(err)
		}
	}

	// Collections past grace keep the Volume's chunk with its bytes.
	for range 2 {
		time.Sleep(grace)
		collect()
	}
	if !stored(shared, sharedData) {
		t.Fatal("a collection freed the shared chunk the fence copy held")
	}

	// The file-only chunk reads as missing through the copy path.
	src := a.pages.GetCopySource()
	got, err := src.GetStoredBlock(ctx, only)
	if err != nil || got != nil {
		t.Fatalf("changed file-only chunk read %v, %v; want a miss", got, err)
	}
	if files.HashMisses.Load() == 0 {
		t.Fatal("the read missed before the hash check")
	}

	// A copy that reads every block again fails on it and stores nothing.
	err = a.copyAndHold(ctx, fenceRootPrefix+"recheck", []*block.BlockRef{parent}, block.GraphCopyOptions{Forget: true}, true)
	if !block.IsNotAvailable(err) {
		t.Fatalf("recheck copy over a changed file = %v, want not available", err)
	}
	if stored(only, nil) {
		t.Fatal("the recheck copy stored the changed chunk")
	}
}

// linkedBucket is a bucket whose copy source reads its linked files after it,
// as the local provider's copy path reads them after the Volume's tiers.
type linkedBucket struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
	// files serves the linked files.
	files *block_store_filesegment.Store
}

// GetCopySource reads the bucket, then the linked files.
func (s *linkedBucket) GetCopySource() block.StoreOps {
	return block_store.NewStoreReadThrough(
		func() block.StoreOps { return s.StoreOps },
		func() block.StoreOps { return s.files.GetCopyTier() },
		nil,
	)
}

// TestAdmitRepeatRewritesTree holds an admitted payload P -> L whose blocks
// a collection freed, as after the stage that stored them was released,
// through the adapter's payload hold with frees on. The hold is refused, and
// a repeat that keeps the tree in the pages again, as a transaction written
// again does, lands the tree in the hold's own batch and commits. Collections
// past grace keep every block.
func TestAdmitRepeatRewritesTree(t *testing.T) {
	ctx := t.Context()
	const grace = 300 * time.Millisecond
	a, collect := newVolumeAdapter(t, 1<<20, grace)
	store := a.pages.StoreOps
	entry := func(name string, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}
	l := entry("leaf")
	p := entry("payload", l.Ref)
	stored := func() bool {
		t.Helper()
		for _, e := range []*block.PutBatchEntry{p, l} {
			if _, found, err := store.GetBlock(ctx, e.Ref); err != nil || !found {
				return false
			}
		}
		return true
	}

	// Store the tree with no holder, as a stage released before the admit
	// leaves it, and let the release's downs age past grace: the next
	// collection nets the tree to zero and frees it in the same pass. A
	// released stage's downs land with the Volume's next changing
	// transaction, so a tick lands them before the wait.
	putUnheld(t, store, l, p)
	putUnheld(t, store, entry("tick"))
	time.Sleep(grace)
	collect()
	if stored() {
		t.Fatal("a collection past grace kept a tree nothing holds, so it checked no free")
	}

	// The hold over the freed tree is refused, then a repeat that keeps the
	// tree in the pages again stores it in the hold's own batch and commits.
	adm := func() *spaceAdmission {
		return &spaceAdmission{payloads: []*block.BlockRef{p.Ref}, done: make(chan struct{})}
	}
	if err := a.holdAdmitted(ctx, adm(), false); !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("hold over a tree at zero = %v, want ErrRootNotHeld", err)
	}
	if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{l, p}); err != nil {
		t.Fatal(err)
	}
	if err := a.holdAdmitted(ctx, adm(), false); err != nil {
		t.Fatalf("repeat: %v", err)
	}

	// Collections past grace keep the tree.
	for range 2 {
		time.Sleep(grace)
		collect()
	}
	if !stored() {
		t.Fatal("a collection freed the admitted tree")
	}
}

// TestAdmitPassesLostPayloadAtBound holds a payload root whose block the
// store lost, as a written-off hole leaves it, in the payload set of an
// adapter whose Volume frees, then admits more payloads than the set's chain
// holds before it rewrites the set as one block. Each admit requires only its
// own payload held, so the rewrite at the bound keeps the lost root and every
// admit commits.
func TestAdmitPassesLostPayloadAtBound(t *testing.T) {
	ctx := t.Context()
	a, _ := newVolumeAdapter(t, 1<<20, time.Hour)
	lost, err := block.BuildBlockRef([]byte("lost payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.addPayloads(ctx, []*block.BlockRef{lost}, false); err != nil {
		t.Fatal(err)
	}
	for i := range maxRootLinks + 1 {
		data := []byte("payload " + strconv.Itoa(i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.pages.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}}); err != nil {
			t.Fatal(err)
		}
		adm := &spaceAdmission{payloads: []*block.BlockRef{ref}, done: make(chan struct{})}
		if err := a.holdAdmitted(ctx, adm, false); err != nil {
			t.Fatalf("admit %d with %d links: %v", i, a.payloads.links, err)
		}
	}
	if !a.payloads.holds(lost) || a.payloads.links > maxRootLinks {
		t.Fatalf("payload set holds the lost root %v with %d links, want held within %d", a.payloads.holds(lost), a.payloads.links, maxRootLinks)
	}
}

// TestAdmitHoldsOverflow admits candidates whose put stores a block past the
// PendingStore's bound on a Volume that frees once a record is grace old, and
// checks that collections past grace leave every block under the candidate's
// root readable from the Volume: one admission whose overflow leaves the kept
// pages under half the bound, and one whose hold waits on the root lock while
// the collections run.
func TestAdmitHoldsOverflow(t *testing.T) {
	const (
		bound = 64 << 10
		grace = 300 * time.Millisecond
	)

	// page returns the entry of a block of at least size bytes referencing
	// children.
	page := func(t *testing.T, name string, size int, children ...*block.BlockRef) *block.PutBatchEntry {
		t.Helper()
		data := []byte(name)
		for _, c := range children {
			data = append(data, c.MarshalString()...)
		}
		if len(data) < size {
			data = append(data, bytes.Repeat([]byte{'.'}, size-len(data))...)
		}
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
	}

	// admission returns an admission of a candidate at the last entry.
	admission := func(entries ...*block.PutBatchEntry) *spaceAdmission {
		root := entries[len(entries)-1].Ref
		return &spaceAdmission{candidate: world.Candidate{Root: root, Entries: entries}, done: make(chan struct{})}
	}

	// age collects past grace often enough to net and free an unheld block.
	age := func(collect func()) {
		for range 3 {
			time.Sleep(grace)
			collect()
		}
	}

	// requireStored fails unless the Volume serves every entry.
	requireStored := func(t *testing.T, a *Adapter, entries ...*block.PutBatchEntry) {
		t.Helper()
		for _, e := range entries {
			_, ok, err := a.pages.StoreOps.GetBlock(t.Context(), e.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("a collection freed block %q of the admitted World", e.Data[:4])
			}
		}
	}

	t.Run("overflow with Full false", func(t *testing.T) {
		a, collect := newVolumeAdapter(t, bound, grace)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, leaf.Ref)
		if err := a.keepAdmitted(t.Context(), admission(leaf, root)); err != nil {
			t.Fatal(err)
		}
		if a.pages.Full() {
			t.Fatal("the kept pages reached half the bound")
		}
		age(collect)
		requireStored(t, a, leaf, root)
	})

	t.Run("hold delayed past grace", func(t *testing.T) {
		a, collect := newVolumeAdapter(t, bound, grace)
		mid := page(t, "mid ", bound/2+1)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, mid.Ref, leaf.Ref)

		// Admit while the test holds the root lock, so the hold waits.
		unlock, err := a.rootMtx.Lock(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() { errCh <- a.keepAdmitted(t.Context(), admission(mid, leaf, root)) }()

		// Wait for the leaf's page, failing at once if the admission returns
		// before its hold, or if the page never arrives.
		deadline := time.Now().Add(10 * time.Second)
		for {
			ok, err := a.pages.StoreOps.GetBlockExists(t.Context(), leaf.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				break
			}
			select {
			case err := <-errCh:
				t.Fatalf("admission returned %v before the leaf's page was kept", err)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("the leaf's page was not kept within 10s")
			}
			time.Sleep(time.Millisecond)
		}
		if !a.pages.Full() {
			t.Fatal("the kept pages stayed under half the bound")
		}

		// Collect past grace while the hold waits, then let it proceed.
		age(collect)
		unlock()
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
		age(collect)
		requireStored(t, a, mid, leaf, root)
	})

	t.Run("overflow stage opened under a cancelled context", func(t *testing.T) {
		// The put past the bound opens the lazy stage under a context
		// already cancelled, and the store's open honours the context,
		// so the open fails. The admission fails with the open and
		// stores nothing the stage would have held: a block the Volume
		// stored unheld would free before any root referenced it. A
		// repeat under a live context admits the World, and collections
		// keep it.
		a, collect := newVolumeAdapter(t, bound, grace)
		inner := a.pages.StoreOps
		a.pages = block.NewPendingStore(&cancelStageStore{
			StoreOps:    inner,
			LeasedRoots: inner.(block.LeasedRoots),
			RootProofs:  inner.(block.RootProofs),
		}, bound)
		leaf := page(t, "leaf", bound+1)
		root := page(t, "root", 0, leaf.Ref)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := a.keepAdmitted(ctx, admission(leaf, root)); !errors.Is(err, context.Canceled) {
			t.Fatalf("admission under a cancelled context = %v, want it cancelled", err)
		}
		for _, e := range []*block.PutBatchEntry{leaf, root} {
			if ok, err := a.pages.StoreOps.GetBlockExists(t.Context(), e.Ref); err != nil || ok {
				t.Fatalf("block %q stored=%v err %v after a cancelled admission, want absent", e.Data[:4], ok, err)
			}
		}
		if err := a.keepAdmitted(t.Context(), admission(leaf, root)); err != nil {
			t.Fatal(err)
		}
		age(collect)
		requireStored(t, a, leaf, root)
	})
}

// cancelStageStore is a bucket whose OpenStage fails under a cancelled
// context, as a stage open that waits on a lock does.
type cancelStageStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs
}

// OpenStage fails under a cancelled context, and otherwise opens the stage.
func (s *cancelStageStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return s.LeasedRoots.OpenStage(ctx)
}

// putUnheld stores entries on store and leaves nothing holding them, as a
// writer that abandons its stage does: a Volume that frees refuses a raw put
// no root holds, so the put goes under a stage released at once.
func putUnheld(t *testing.T, store block.StoreOps, entries ...*block.PutBatchEntry) {
	t.Helper()
	staged, release, err := block.OpenStage(t.Context(), store)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := staged.PutBlockBatch(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
}

// TestQueuedRootMoves checks that queued admission root moves never let a
// stored root name a block the collector freed, nor a state outlive its holds
// in a crash: a drop fails every admission that can rely on the holds it
// released and spares one whose moves already landed, a landing in the turn
// survives its cancellation, and every landing commits as the adapter's holds
// require, whichever lander runs it.
func TestQueuedRootMoves(t *testing.T) {
	// A drop between holdAdmitted's payload move and its candidate move, which
	// take the root lock separately while a lander drops without it, removes
	// the payload move. The candidate move must be refused.
	t.Run("between payload and candidate", func(t *testing.T) {
		ctx := t.Context()
		a, collect, p, release := newDropAdapter(t, "payload-between-drop-and-candidate")
		c := putDropPage(t, a, "candidate-naming-", p)
		adm := a.newAdmission(world.Candidate{Root: c}, nil, []*block.BlockRef{p}, nil)
		if err := a.moves.join(&adm.moves, nil); err != nil {
			t.Fatal(err)
		}
		qctx := withQueuing(ctx, &adm.moves)
		if err := a.addPayloads(qctx, adm.payloads, true); err != nil {
			t.Fatal(err)
		}
		dropFreed(t, a, collect, p, release, &adm.moves)
		if err := addPending(qctx, a, c); err == nil {
			t.Fatal("candidate move queued after its payload move was dropped")
		}
		if err := a.moves.depend(&adm.moves, nil); err == nil {
			t.Fatal("admission survived the drop of its payload move")
		}
		requirePendingWhole(t, a)
	})

	// A staged admission whose payload scope finds the payload held by
	// another admission's queued move borrows that hold and moves nothing. A
	// drop before its candidate scope must fail it.
	t.Run("staged borrow", func(t *testing.T) {
		ctx := t.Context()
		a, collect, p, release := newDropAdapter(t, "payload-shared-by-two-admissions")
		cb := putDropPage(t, a, "staged-candidate-", p)
		admA := a.newAdmission(world.Candidate{}, nil, []*block.BlockRef{p}, nil)
		if err := a.moves.join(&admA.moves, nil); err != nil {
			t.Fatal(err)
		}
		if err := a.addPayloads(withQueuing(ctx, &admA.moves), admA.payloads, true); err != nil {
			t.Fatal(err)
		}
		if err := a.moves.depend(&admA.moves, nil); err != nil {
			t.Fatal(err)
		}

		admB := a.newAdmission(world.Candidate{Root: cb, Staged: true}, nil, []*block.BlockRef{p}, nil)
		if err := a.moves.join(&admB.moves, nil); err != nil {
			t.Fatal(err)
		}
		bctx := withQueuing(ctx, &admB.moves)
		if err := a.addPayloads(bctx, admB.payloads, true); err != nil {
			t.Fatal(err)
		}
		if !a.moves.queued(&admA.moves) {
			t.Fatal("the staged payload scope landed the move it borrows")
		}
		dropFreed(t, a, collect, p, release, &admA.moves)
		addErr := addPending(bctx, a, cb)
		if err := a.moves.depend(&admB.moves, addErr); err == nil {
			t.Fatal("staged admission survived the drop of the hold it borrowed")
		}
		requirePendingWhole(t, a)
	})

	// An admission built on a candidate whose overflow stage a drop released
	// must fail, since its World reaches the pages that stage held.
	t.Run("successor of dropped overflow", func(t *testing.T) {
		const bound = 64 << 10
		ctx := t.Context()
		a, collect, p, release := newDropAdapter(t, "payload-of-the-base-only")
		leaf := dropPage(t, "leaf", bound+1)
		rootA := dropPage(t, "rootA", 0, leaf.Ref)
		cand := world.Candidate{Root: rootA.Ref, Entries: []*block.PutBatchEntry{leaf, rootA}}
		admA := a.newAdmission(cand, nil, []*block.BlockRef{p}, nil)
		if err := a.keepAdmitted(ctx, admA); err != nil {
			t.Fatal(err)
		}
		if !a.moves.queued(&admA.moves) {
			t.Fatal("the base's hold did not queue")
		}
		dropFreed(t, a, collect, p, release, &admA.moves)
		for range 3 {
			collect()
		}
		if _, found, err := a.pages.StoreOps.GetBlock(ctx, leaf.Ref); err != nil || found {
			t.Fatalf("overflow leaf not freed: found %v: %v", found, err)
		}

		rootB := dropPage(t, "rootB", 0, leaf.Ref)
		admB := a.newAdmission(world.Candidate{Root: rootB.Ref, Entries: []*block.PutBatchEntry{rootB}}, nil, nil, nil)
		admB.after = admA
		keepErr := a.keepAdmitted(ctx, admB)
		landErr := a.moves.landFor(ctx, a.pages, &admB.moves)
		if keepErr == nil && landErr == nil {
			t.Fatal("successor accepted while its World names a freed page")
		}
		requirePendingWhole(t, a)
	})

	// An admission whose moves landed relies only on durable roots, so a
	// later drop must leave it able to write its state.
	t.Run("landed owner spared", func(t *testing.T) {
		ctx := t.Context()
		a, collect, p, release := newDropAdapter(t, "payload-of-the-dropped-admission")
		stage, keep, err := block.OpenStage(ctx, a.pages.StoreOps)
		if err != nil {
			t.Fatal(err)
		}
		defer keep()
		kept, _, err := stage.PutBlock(ctx, []byte("payload-of-the-landed-admission"), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		admA := a.newAdmission(world.Candidate{}, nil, []*block.BlockRef{kept}, nil)
		if err := a.keepAdmitted(ctx, admA); err != nil {
			t.Fatal(err)
		}
		if !a.moves.queued(&admA.moves) {
			t.Fatal("the landed admission's hold did not queue")
		}
		if err := a.landQueued(ctx, admA); err != nil {
			t.Fatal(err)
		}

		admB := a.newAdmission(world.Candidate{}, nil, []*block.BlockRef{p}, nil)
		if err := a.keepAdmitted(ctx, admB); err != nil {
			t.Fatal(err)
		}
		dropFreed(t, a, collect, p, release, &admB.moves)
		if err := a.landQueued(ctx, admA); err != nil {
			t.Fatalf("landed admission failed by a later drop: %v", err)
		}
	})

	// A staged admission's landing in the turn, which its context cancels
	// while another lander's SetRoots hangs, must drop its moves once that
	// landing passes its bound, so none stays queued past the writer's stage
	// that holds its drained page and the turn does not wait on the store.
	t.Run("staged landing drops at the deadline", func(t *testing.T) {
		ctx := t.Context()
		a, collect, p, _ := newDropAdapter(t, "payload-held-by-a-hung-landing")
		a.moves.landTimeout = 50 * time.Millisecond
		var hang atomic.Bool
		entered := make(chan struct{}, 1)
		hookRoots(a, func(ctx context.Context, _ []block.NamedRoot) error {
			if !hang.Load() {
				return nil
			}
			select {
			case entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return ctx.Err()
		})

		// Queue a payload hold and start a lander whose SetRoots hangs.
		adm0 := a.newAdmission(world.Candidate{}, nil, []*block.BlockRef{p}, nil)
		if err := a.keepAdmitted(ctx, adm0); err != nil {
			t.Fatal(err)
		}
		hang.Store(true)
		bgErr := make(chan error, 1)
		go func() { bgErr <- a.moves.landAll(ctx, a.pages) }()
		<-entered

		// Cancel a staged turn that waits behind it.
		stage, releaseStage, err := block.OpenStage(ctx, a.pages.StoreOps)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseStage()
		w, _, err := stage.PutBlock(ctx, []byte("page-drained-to-the-writer-stage"), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		adm := a.newAdmission(world.Candidate{Root: w, Staged: true}, nil, nil, nil)
		cctx, cancel := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() { errCh <- a.keepAdmitted(cctx, adm) }()
		for !a.moves.queued(&adm.moves) {
			select {
			case err := <-errCh:
				t.Fatalf("staged move never queued: %v", err)
			case <-time.After(time.Millisecond):
			}
		}
		cancel()

		// Both landings fail at the deadline with the queue dropped.
		select {
		case err := <-errCh:
			if err == nil {
				t.Fatal("staged admission accepted although its landing was dropped")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the staged turn waited past the landing bound")
		}
		if err := <-bgErr; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("hung landing returned %v, want its deadline", err)
		}
		if a.moves.queued(&adm.moves) || a.moves.queued(&adm0.moves) {
			t.Fatal("a move stayed queued past the landing bound")
		}
		hang.Store(false)

		releaseStage()
		for range 3 {
			collect()
		}
		requirePendingWhole(t, a)
	})

	// A staged admission whose landing in the turn fails drops its moves. An
	// admission built on its candidate must fail, and no pending root may name
	// a freed block once the stage ends.
	t.Run("staged drop fails successor", func(t *testing.T) {
		ctx := t.Context()
		a, collect, _, _ := newDropAdapter(t, "payload-the-staged-admission-skips")
		errInjected := errors.New("injected SetRoots failure")
		var fail atomic.Bool
		hookRoots(a, func(context.Context, []block.NamedRoot) error {
			if fail.Load() {
				return errInjected
			}
			return nil
		})
		stage, releaseStage, err := block.OpenStage(ctx, a.pages.StoreOps)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseStage()
		w, _, err := stage.PutBlock(ctx, []byte("page-drained-to-the-writer-stage"), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
		admA := a.newAdmission(world.Candidate{Root: w, Staged: true}, nil, nil, nil)
		fail.Store(true)
		errA := a.keepAdmitted(ctx, admA)
		fail.Store(false)
		if !errors.Is(errA, errInjected) {
			t.Fatalf("staged admission with a failed landing returned %v", errA)
		}
		if a.moves.queued(&admA.moves) {
			t.Fatal("failed staged landing left its move queued")
		}

		// The successor's World reaches w through a new root page.
		rootB := dropPage(t, "rootB", 0, w)
		admB := a.newAdmission(world.Candidate{Root: rootB.Ref, Entries: []*block.PutBatchEntry{rootB}}, nil, nil, nil)
		admB.after = admA
		errB := a.keepAdmitted(ctx, admB)
		if errB == nil {
			errB = a.landQueued(ctx, admB)
		}
		if !errors.Is(errB, errInjected) {
			t.Fatalf("successor of a failed staged admission returned %v", errB)
		}

		releaseStage()
		for range 3 {
			collect()
		}
		requirePendingWhole(t, a)
	})

	// Without ordered holds, an admission's queued hold must commit flushed
	// before its state write, even when a background root move, whose context
	// asks for an ordered commit, lands it first.
	t.Run("background landing flushes", func(t *testing.T) {
		ctx := t.Context()
		a, _, p, _ := newDropAdapter(t, "payload-held-by-an-upload-stage")
		var mtx sync.Mutex
		flushed := make(map[string]bool)
		hookRoots(a, func(ctx context.Context, roots []block.NamedRoot) error {
			if !kvtx.IsOrderedCommitContext(ctx) {
				mtx.Lock()
				for _, r := range roots {
					flushed[r.Name] = true
				}
				mtx.Unlock()
			}
			return nil
		})
		if a.orderedHolds {
			t.Fatal("adapter orders holds")
		}
		adm := a.newAdmission(world.Candidate{}, nil, []*block.BlockRef{p}, nil)
		if err := a.keepAdmitted(ctx, adm); err != nil {
			t.Fatal(err)
		}
		if !a.moves.queued(&adm.moves) {
			t.Fatal("payload hold did not queue")
		}

		e := dropPage(t, "background-root", 0)
		bctx := kvtx.WithOrderedCommitContext(ctx)
		if err := a.moveRoots(bctx, []*block.PutBatchEntry{e}, block.NamedRoot{Name: RootBackfill, Ref: e.Ref}); err != nil {
			t.Fatal(err)
		}
		if err := a.landQueued(ctx, adm); err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		defer mtx.Unlock()
		if !flushed[a.payloads.name] {
			t.Fatal("the payload hold committed only ordered before its state write")
		}
	})
}

// newDropAdapter opens an adapter that queues its admission moves, and a
// payload that only the returned stage release holds.
func newDropAdapter(t *testing.T, payload string) (*Adapter, func(), *block.BlockRef, func()) {
	t.Helper()
	ctx := t.Context()
	a, collect := newVolumeAdapter(t, 64<<10, time.Nanosecond)
	a.deferMoves = true
	stage, release, err := block.OpenStage(ctx, a.pages.StoreOps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	p, _, err := stage.PutBlock(ctx, []byte(payload), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	return a, collect, p, release
}

// dropPage returns an entry of at least size bytes referencing children.
func dropPage(t *testing.T, name string, size int, children ...*block.BlockRef) *block.PutBatchEntry {
	t.Helper()
	data := []byte(name)
	for _, c := range children {
		data = append(data, c.MarshalString()...)
	}
	if len(data) < size {
		data = append(data, bytes.Repeat([]byte{'.'}, size-len(data))...)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: children, RefsKnown: true}
}

// putDropPage stores a page naming child on a's kept pages.
func putDropPage(t *testing.T, a *Adapter, name string, child *block.BlockRef) *block.BlockRef {
	t.Helper()
	e := dropPage(t, name, 0, child)
	if _, err := a.pages.PutBlockBatch(t.Context(), []*block.PutBatchEntry{e}); err != nil {
		t.Fatal(err)
	}
	return e.Ref
}

// dropFreed releases the stage holding p, collects p, and lands owner's
// moves, which the strict move of p makes the store refuse and drop.
func dropFreed(t *testing.T, a *Adapter, collect func(), p *block.BlockRef, release func(), owner *admissionMoves) {
	t.Helper()
	ctx := t.Context()
	release()
	for range 3 {
		collect()
	}
	if _, found, err := a.pages.StoreOps.GetBlock(ctx, p); err != nil || found {
		t.Fatalf("payload not freed: found %v: %v", found, err)
	}
	if err := a.moves.landFor(ctx, a.pages, owner); !errors.Is(err, block.ErrRootNotHeld) {
		t.Fatalf("landing the dropped payload: %v", err)
	}
}

// addPending moves the pending Worlds root to hold root, as a candidate scope
// does.
func addPending(ctx context.Context, a *Adapter, root *block.BlockRef) error {
	unlock, err := a.lockRoots(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return a.addRoot(ctx, &a.pendingWorlds, root)
}

// requirePendingWhole lands every queued move and fails when the stored
// pending Worlds root names a freed block.
func requirePendingWhole(t *testing.T, a *Adapter) {
	t.Helper()
	ctx := t.Context()
	if err := a.moves.landAll(ctx, a.pages); err != nil {
		t.Fatal(err)
	}
	store := a.pages.StoreOps
	root, err := store.GetRoot(ctx, RootPending)
	if err != nil {
		t.Fatal(err)
	}
	if root != nil {
		if err := block.WalkGraph(ctx, store, []*block.BlockRef{root}, 0, nil); err != nil {
			t.Fatalf("pending root names a freed block: %v", err)
		}
	}
}

// rootsHookStore calls hook before each SetRoots of the wrapped store, and
// moves the roots only when hook returns nil.
type rootsHookStore struct {
	block.StoreOps
	block.LeasedRoots
	block.RootProofs

	hook func(ctx context.Context, roots []block.NamedRoot) error
}

// OpenStage opens a stage of the wrapped store.
func (s *rootsHookStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	return s.LeasedRoots.OpenStage(ctx)
}

// SetRoots moves the roots once hook accepts them.
func (s *rootsHookStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if err := s.hook(ctx, roots); err != nil {
		return err
	}
	return s.StoreOps.SetRoots(ctx, entries, roots)
}

// hookRoots makes a's pending store call hook before each SetRoots.
func hookRoots(a *Adapter, hook func(ctx context.Context, roots []block.NamedRoot) error) {
	inner := a.pages.StoreOps
	a.pages = block.NewPendingStore(&rootsHookStore{
		StoreOps:    inner,
		LeasedRoots: inner.(block.LeasedRoots),
		RootProofs:  inner.(block.RootProofs),
		hook:        hook,
	}, 64<<10)
}

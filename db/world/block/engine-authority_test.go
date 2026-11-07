//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestEngineAuthorityRunThrough walks the World engine's commit through its
// Authority on a Volume that frees at once: a commit is one Volume
// transaction that moves the World root; a writer that loses a race or its
// lease is superseded and rebases; another engine follows the head; an
// authority that answers later holds the candidate under a pending root
// across a restart; collection keeps what the head reaches and frees what it
// does not.
func TestEngineAuthorityRunThrough(t *testing.T) {
	f := newVolumeFixture(t)
	ctx := t.Context()

	// A commit is one Volume transaction, and the World root holds its head.
	t.Run("commit", func(t *testing.T) {
		// Warm the writer's path, then measure one commit.
		w := fixtureWriter(t, f.engine)
		fixtureObject(t, w, "warm", "warm")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		before := f.seq()
		w = fixtureWriter(t, f.engine)
		fixtureObject(t, w, "a", "a-1")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if n := f.seq() - before; n != 1 {
			t.Fatalf("one commit ran %d Volume transactions, want 1", n)
		}

		// The root, the authority and the engine agree on the head.
		root := f.engine.GetRootRef().GetRootRef()
		if !f.worldRoot(t).EqualsRef(root) {
			t.Fatal("the World root does not hold the published head")
		}
		head, err := f.engine.authority.Head(ctx)
		if err != nil || !head.Root.EqualsRef(root) || !head.Durable {
			t.Fatalf("authority head %v %v disagrees with the engine", head, err)
		}
	})

	// A writer that loses a race to another engine's writer is superseded,
	// stores nothing, and rebases onto the winner's head.
	t.Run("race", func(t *testing.T) {
		// Open a second engine on its own authority without the coordinator,
		// as a writer that does not take the lease.
		other := f.openEngine(t, NewLocalAuthority(f.le, f.base, fixtureRootName, nil, f.engine.authority.(*LocalAuthority).scope))

		// Open a writer here, let the other engine commit first.
		w := fixtureWriter(t, f.engine)
		lost := fixtureObject(t, w, "race", "race-lost")
		ow := fixtureWriter(t, other)
		fixtureObject(t, ow, "race", "race-won")
		if err := ow.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		won := f.worldRoot(t)

		// The writer's swap finds the winner's root.
		if err := w.Commit(ctx); !errors.Is(err, world.ErrSuperseded) {
			t.Fatalf("losing commit returned %v, want ErrSuperseded", err)
		}
		if !f.worldRoot(t).EqualsRef(won) {
			t.Fatal("a superseded commit moved the World root")
		}
		if f.stored(t, lost.GetRootRef()) {
			t.Fatal("a superseded commit stored its blocks")
		}

		// The rebased writer builds on the winner and commits.
		w = fixtureWriter(t, f.engine)
		fixtureHas(t, w, "race", true)
		fixtureObject(t, w, "race", "race-rebased")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if !f.worldRoot(t).EqualsRef(f.engine.GetRootRef().GetRootRef()) {
			t.Fatal("the rebased commit's head is not the World root")
		}

		// A write through world.ExecTransaction that loses the race on its
		// first attempt rebases and lands on its second.
		var attempts int
		err := world.ExecTransaction(ctx, f.engine, true, func(ctx context.Context, ws world.WorldState) error {
			attempts++
			if attempts == 1 {
				ow := fixtureWriter(t, other)
				fixtureObject(t, ow, "race-exec-other", "race-exec-other")
				if err := ow.Commit(ctx); err != nil {
					return err
				}
			}
			obj, err := ws.CreateObject(ctx, "race-exec", nil)
			world.ReleaseObjectState(obj)
			return err
		})
		if err != nil || attempts != 2 {
			t.Fatalf("ExecTransaction = %v after %d attempts, want success after 2", err, attempts)
		}
		r := fixtureWriter(t, f.engine)
		fixtureHas(t, r, "race-exec", true)
		fixtureHas(t, r, "race-exec-other", true)
		r.Discard()
	})

	// A writer whose lease is lost mid-commit, while another engine takes the
	// lease and commits, is superseded by the swap's Previous.
	t.Run("lost lease", func(t *testing.T) {
		// Open a second engine sharing the coordinator, as another process.
		other := f.openEngine(t, f.authority())

		// Open a writer here and lose its lease.
		w := fixtureWriter(t, f.engine)
		fixtureObject(t, w, "lease", "lease-lost")
		f.coordinator.lose(t)

		// The other engine takes the lease and commits.
		ow := fixtureWriter(t, other)
		fixtureObject(t, ow, "lease", "lease-won")
		if err := ow.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// The writer's commit is superseded, and the head and root agree.
		if err := w.Commit(ctx); !errors.Is(err, world.ErrSuperseded) {
			t.Fatalf("commit after a lost lease returned %v, want ErrSuperseded", err)
		}
		w = fixtureWriter(t, f.engine)
		if !w.baseHeadRef.GetRootRef().EqualsRef(f.worldRoot(t)) {
			t.Fatal("the next writer does not build on the World root")
		}
		w.Discard()
	})

	// Another engine on the Volume follows each commit through its
	// authority's head watch, without opening a writer.
	t.Run("follow", func(t *testing.T) {
		// Open a follower and commit here.
		follower := f.openEngine(t, f.authority())
		w := fixtureWriter(t, f.engine)
		fixtureObject(t, w, "follow", "follow-1")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// The follower reaches the commit's seqno at its root.
		seqno, err := f.engine.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if _, err := follower.WaitSeqno(waitCtx, seqno); err != nil {
			t.Fatal(err)
		}
		if !follower.GetRootRef().EqualsRef(f.engine.GetRootRef()) {
			t.Fatal("the follower's head differs from the writer's")
		}

		// A settings change moves the root and adds no change log entry, so
		// the head keeps its seqno; the follower installs it anyway.
		if err := f.engine.SetChangelogDisabled(ctx, true); err != nil {
			t.Fatal(err)
		}
		after, err := f.engine.GetSeqno(ctx)
		if err != nil || after != seqno {
			t.Fatalf("settings commit seqno %d, %v; want the unchanged %d", after, err, seqno)
		}
		want := f.engine.GetRootRef()
		for !follower.GetRootRef().EqualsRef(want) {
			changed := follower.headSignal.Load().changed
			if follower.GetRootRef().EqualsRef(want) {
				break
			}
			select {
			case <-changed:
			case <-waitCtx.Done():
				t.Fatal("the follower missed a head at an unchanged seqno")
			}
		}
	})

	// An authority that answers later holds the candidate's blocks under a
	// pending root until the head is durable, and across a restart.
	t.Run("pending", func(t *testing.T) {
		// Open an engine on an authority that answers later.
		pending := newPendingAuthority(f.authority())
		engine := f.openEngine(t, pending)
		before := engine.GetRootRef()

		// Commit; the candidate is admitted and waits under its pending root.
		w := fixtureWriter(t, engine)
		payload := fixtureObject(t, w, "pending", "pending-1")
		done := make(chan error, 1)
		go func() { done <- w.Commit(ctx) }()
		adm := pending.waitAdmitted(t)
		pendingRoot, err := f.store.GetRoot(ctx, pendingRootName(adm.candidate.Seqno))
		if err != nil || !pendingRoot.EqualsRef(adm.candidate.Root) {
			t.Fatalf("pending root %v %v does not hold the candidate", pendingRoot, err)
		}

		// Readers keep the durable head, and collection keeps the candidate.
		if !engine.GetRootRef().EqualsRef(before) || !f.worldRoot(t).EqualsRef(before.GetRootRef()) {
			t.Fatal("an undurable candidate became the head")
		}
		f.collect(t)
		if !f.stored(t, payload.GetRootRef()) {
			t.Fatal("collection freed a pending candidate's block")
		}

		// The head becomes durable in one swap that releases the pending root.
		pending.durable(t)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !f.worldRoot(t).EqualsRef(engine.GetRootRef().GetRootRef()) {
			t.Fatal("the durable head is not the World root")
		}
		if ref, _ := f.store.GetRoot(ctx, pendingRootName(adm.candidate.Seqno)); ref != nil {
			t.Fatal("the durable head kept its pending root")
		}

		// A candidate the authority made durable is a success even when the
		// report of its record is lost: the engine reconciles with the head.
		w = fixtureWriter(t, engine)
		fixtureObject(t, w, "pending-lost-report", "pending-lost-report")
		go func() { done <- w.Commit(ctx) }()
		pending.durableLost(t, errors.New("record report lost"))
		if err := <-done; err != nil {
			t.Fatalf("a durable commit whose report was lost returned %v", err)
		}
		if !f.worldRoot(t).EqualsRef(engine.GetRootRef().GetRootRef()) {
			t.Fatal("the reconciled head is not the published head")
		}

		// A candidate whose authority stops before it is durable keeps its
		// pending root across the restart, and the restarted engine opens
		// at the durable head.
		w = fixtureWriter(t, engine)
		stranded := fixtureObject(t, w, "pending", "pending-stranded")
		go func() { done <- w.Commit(ctx) }()
		adm = pending.fail(t, errors.New("authority stopped"))
		if err := <-done; !world.IsSuperseded(err) {
			t.Fatalf("a commit whose authority stopped returned %v, want superseded", err)
		}
		durable := f.worldRoot(t)
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
		restarted := f.openEngine(t, newPendingAuthority(f.authority()))
		if !restarted.GetRootRef().GetRootRef().EqualsRef(durable) {
			t.Fatal("the restarted engine is not at the durable head")
		}
		f.collect(t)
		if !f.stored(t, stranded.GetRootRef()) {
			t.Fatal("collection freed a block a pending root holds")
		}

		// Releasing the stranded pending root frees its candidate.
		name := pendingRootName(adm.candidate.Seqno)
		if err := f.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: name, Previous: adm.candidate.Root}}); err != nil {
			t.Fatal(err)
		}
		f.collect(t)
		if f.stored(t, stranded.GetRootRef()) {
			t.Fatal("a released pending candidate's block survived collection")
		}
	})

	// An authority that pipelines lets a writer begin on the previous
	// writer's admitted candidate before it is durable: the later commit's
	// Admit returns while the earlier one waits, readers see each head only
	// once it is decided, and a head the authority rebased is published in
	// the candidate's place.
	t.Run("pipelined", func(t *testing.T) {
		// Open an engine on a pipelining authority at the World root.
		head, err := f.engine.authority.Head(ctx)
		if err != nil {
			t.Fatal(err)
		}
		pipe := newPipeAuthority(f.store, head)
		engine := f.openEngine(t, pipe)
		published := func(key string, want bool) {
			t.Helper()
			r, err := engine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Discard()
			fixtureHas(t, r, key, want)
		}

		// The first writer's commit waits for its decision.
		w1 := fixtureWriter(t, engine)
		fixtureObject(t, w1, "pipe-1", "pipe-1")
		done1 := make(chan error, 1)
		go func() { done1 <- w1.Commit(ctx) }()
		pipe.waitAdmitted(t, 1)

		// The second writer begins on the first candidate, and its Admit
		// returns while the first is not durable.
		w2 := fixtureWriter(t, engine)
		fixtureHas(t, w2, "pipe-1", true)
		fixtureObject(t, w2, "pipe-2", "pipe-2")
		done2 := make(chan error, 1)
		go func() { done2 <- w2.Commit(ctx) }()
		pipe.waitAdmitted(t, 2)
		published("pipe-1", false)

		// Each commit returns once decided, with its head published.
		pipe.decide(t, 1, nil, 0)
		if err := <-done1; err != nil {
			t.Fatal(err)
		}
		published("pipe-1", true)
		published("pipe-2", false)
		pipe.decide(t, 2, nil, 0)
		if err := <-done2; err != nil {
			t.Fatal(err)
		}
		published("pipe-2", true)

		// A pipelined commit the authority rebases onto another writer's
		// change publishes the rebased head.
		w3 := fixtureWriter(t, engine)
		fixtureObject(t, w3, "pipe-3", "pipe-3")
		done3 := make(chan error, 1)
		var committed3 *bucket.ObjectRef
		go func() {
			var err error
			committed3, err = w3.CommitBlockTransaction(ctx)
			done3 <- err
		}()
		adm3 := pipe.waitAdmitted(t, 3)
		root := f.base.Clone()
		defer root.Release()
		root.SetRootRef(adm3.candidate.Root)
		other, err := NewEngine(ctx, f.le, root, world_mock.LookupMockOp, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		ow := fixtureWriter(t, other)
		fixtureObject(t, ow, "pipe-rebase", "pipe-rebase")
		rebased, err := ow.CommitBlockTransaction(ctx)
		if err != nil {
			t.Fatal(err)
		}
		seqno, err := other.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := other.Close(); err != nil {
			t.Fatal(err)
		}
		pipe.decide(t, 3, rebased.GetRootRef(), seqno)
		if err := <-done3; err != nil {
			t.Fatal(err)
		}
		if !committed3.GetRootRef().EqualsRef(rebased.GetRootRef()) || !engine.GetRootRef().GetRootRef().EqualsRef(rebased.GetRootRef()) {
			t.Fatal("the rebased head was not published as the commit's head")
		}
		published("pipe-3", true)
		published("pipe-rebase", true)

		// A head the authority reports while a local writer holds its turn
		// reaches readers at once, and the writer still commits.
		w4 := fixtureWriter(t, engine)
		fixtureObject(t, w4, "pipe-4", "pipe-4")
		root.SetRootRef(engine.GetRootRef().GetRootRef())
		other, err = NewEngine(ctx, f.le, root, world_mock.LookupMockOp, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		ow = fixtureWriter(t, other)
		fixtureObject(t, ow, "pipe-remote", "pipe-remote")
		remote, err := ow.CommitBlockTransaction(ctx)
		if err == nil {
			seqno, err = other.GetSeqno(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := other.Close(); err != nil {
			t.Fatal(err)
		}
		pipe.report(remote.GetRootRef(), seqno)
		published("pipe-remote", true)
		done4 := make(chan error, 1)
		go func() { done4 <- w4.Commit(ctx) }()
		pipe.decide(t, 4, nil, 0)
		if err := <-done4; err != nil {
			t.Fatal(err)
		}
		published("pipe-4", true)
	})

	// A reader that holds a replaced head past the pin delay has it pinned,
	// and reads it after a collection; one that lets go first leaves no pin.
	t.Run("reader pin", func(t *testing.T) {
		// pinned reports whether the Volume holds a pin of root.
		delay := 20 * time.Millisecond
		engine := f.openEngine(t, f.authority(), WithReaderPinDelay(delay))
		pinned := func(root *block.BlockRef) bool {
			t.Helper()
			var found bool
			err := f.volume.ListRoots(ctx, "pin:", func(r block.NamedRoot) error {
				found = found || r.Ref.EqualsRef(root)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			return found
		}
		commit := func(key string) {
			t.Helper()
			w := fixtureWriter(t, engine)
			fixtureObject(t, w, key, key)
			if err := w.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}

		// Hold a reader on a head across the commit that replaces it.
		commit("pin-held")
		held := engine.GetRootRef().GetRootRef()
		reader, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		commit("pin-after-held")

		// The pin lands after the delay, and the reader survives a collection.
		deadline := time.Now().Add(5 * time.Second)
		for !pinned(held) {
			if time.Now().After(deadline) {
				t.Fatal("a replaced head a reader holds was never pinned")
			}
			time.Sleep(delay)
		}
		f.collect(t)
		fixtureHas(t, reader, "pin-held", true)
		reader.Discard()

		// A reader that lets go before the delay leaves its head unpinned.
		short := engine.GetRootRef().GetRootRef()
		reader, err = engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		commit("pin-after-short")
		reader.Discard()
		time.Sleep(5 * delay)
		if pinned(short) {
			t.Fatal("a head every reader released before the delay was pinned")
		}
	})

	// A close while a long commit sits in Admit leaves the commit its stage,
	// so its early drains survive a collection until its swap lands.
	t.Run("close during commit", func(t *testing.T) {
		// Hold the commit at Admit.
		entered, proceed := make(chan struct{}), make(chan struct{})
		engine := f.openEngine(t, &hookAuthority{
			Authority: f.authority(),
			admit: func(context.Context, world.Candidate) error {
				close(entered)
				<-proceed
				return nil
			},
		})

		// Fill a writer past the buffer, then commit it.
		w := fixtureWriter(t, engine)
		refs := make([]*bucket.ObjectRef, 0, 4200)
		for _, key := range fixtureKeys("close-", 4200) {
			refs = append(refs, fixtureObject(t, w, key, key))
		}
		if !f.stored(t, refs[0].GetRootRef()) {
			t.Fatal("the writer did not drain early")
		}
		done := make(chan error, 1)
		go func() { done <- w.Commit(ctx) }()
		<-entered

		// Close the engine, collect past the grace, then let the commit swap.
		closed := make(chan error, 1)
		go func() { closed <- engine.Close() }()
		time.Sleep(50 * time.Millisecond)
		f.collect(t)
		close(proceed)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := <-closed; err != nil {
			t.Fatal(err)
		}

		// The commit's early drains are still stored.
		f.collect(t)
		if !f.stored(t, refs[0].GetRootRef()) {
			t.Fatal("a close released a committing writer's stage before its swap")
		}
	})

	// Collection keeps the committed root and frees the blocks no commit
	// reached: a stray block of a commit, and everything of a discarded
	// writer.
	t.Run("unreached writes", func(t *testing.T) {
		// Commit a writer with a stray payload, and discard another.
		w := fixtureWriter(t, f.engine)
		committed := fixtureObject(t, w, "unreached-committed", "unreached-committed")
		stray := fixturePayload(t, w, "unreached-stray")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		d := fixtureWriter(t, f.engine)
		discarded := fixtureObject(t, d, "unreached-discarded", "unreached-discarded")
		d.Discard()

		// Only the committed payload survives collection.
		f.collect(t)
		for ref, want := range map[*bucket.ObjectRef]bool{committed: true, stray: false, discarded: false} {
			if got := f.stored(t, ref.GetRootRef()); got != want {
				t.Fatalf("payload %s stored=%v, want %v", ref.GetRootRef().MarshalString(), got, want)
			}
		}
	})

	// Rounds of creates, links and deletes leave a graph the collector frees
	// garbage from and the audit finds consistent.
	t.Run("world pass", func(t *testing.T) {
		// Commit rounds that create objects, link them, then delete half.
		var freed uint64
		for round := range 8 {
			w := fixtureWriter(t, f.engine)
			for i := range 16 {
				key := "pass-" + strconv.Itoa(round) + "-" + strconv.Itoa(i)
				fixtureObject(t, w, key, key)
				if i != 0 {
					q := world.NewGraphQuadWithKeys("pass-"+strconv.Itoa(round)+"-0", "<link>", key, "")
					if err := w.SetGraphQuad(ctx, q); err != nil {
						t.Fatal(err)
					}
				}
			}
			if round != 0 {
				for i := range 8 {
					if _, err := w.DeleteObject(ctx, "pass-"+strconv.Itoa(round-1)+"-"+strconv.Itoa(i)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := w.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			freed += f.collect(t).freed
		}

		// The collector freed replaced pages, and the audit finds no live
		// block queued or undercounted.
		if freed == 0 {
			t.Fatal("collection freed no replaced block")
		}
		audit, err := f.volume.Audit(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if audit.LiveQueued != 0 || audit.Undercount != 0 {
			t.Fatalf("audit found live blocks at risk: queued=%d undercount=%d", audit.LiveQueued, audit.Undercount)
		}
	})

	// A commit too large for the write buffer drains early under its stage,
	// and keeps every drained block across collections that run past the
	// grace period until its swap; an abandoned one frees them.
	t.Run("long commit", func(t *testing.T) {
		// write fills a writer past the buffer's 4096 entries.
		write := func(prefix string) (*EngineTx, []*bucket.ObjectRef) {
			w := fixtureWriter(t, f.engine)
			refs := make([]*bucket.ObjectRef, 0, 4200)
			for _, key := range fixtureKeys(prefix, 4200) {
				refs = append(refs, fixtureObject(t, w, key, key))
			}
			if !f.stored(t, refs[0].GetRootRef()) {
				t.Fatal("the writer did not drain early")
			}
			return w, refs
		}

		// The committed writer keeps every drained block until and after its
		// swap.
		w, refs := write("long-")
		f.collect(t)
		if !f.stored(t, refs[0].GetRootRef()) {
			t.Fatal("collection freed an early drain of an open commit")
		}
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		f.collect(t)
		for _, ref := range []*bucket.ObjectRef{refs[0], refs[len(refs)-1]} {
			if !f.stored(t, ref.GetRootRef()) {
				t.Fatal("collection freed a committed block")
			}
		}

		// The abandoned writer's drained blocks go once it releases them.
		d, abandoned := write("abandoned-")
		f.collect(t)
		if !f.stored(t, abandoned[0].GetRootRef()) {
			t.Fatal("collection freed an early drain of an open writer")
		}
		d.Discard()
		f.collect(t)
		if f.stored(t, abandoned[0].GetRootRef()) {
			t.Fatal("an abandoned writer's early drain survived collection")
		}
	})

	// A World read and a small commit grow at most with the depth of the
	// object tree from 1x to 10x objects, as the Phase 2 path walk says: a
	// commit stays one Volume transaction, and neither scans the World.
	t.Run("scaling", func(t *testing.T) {
		small, large := measureEnginePaths(t, 200), measureEnginePaths(t, 2000)
		for name, s := range small {
			l := large[name]
			t.Logf("%s: %+v over 1x, %+v over 10x", name, s, l)
			if l.Total() > 2*s.Total()+8 || l.WriteTxns > s.WriteTxns {
				t.Errorf("%s over 10x objects cost %+v, over 1x %+v", name, l, s)
			}
		}
	})
}

// measureEnginePaths opens an engine with a local authority over a kvtx
// Volume on a counting store, writes n objects, and returns what a World read
// of one object and a commit of one object cost. A read's blocks the decoded
// block cache serves count as reads, so the cache does not hide work.
func measureEnginePaths(t *testing.T, n int) map[string]kvtx_kvtest.Counts {
	// Open the Volume, the World's cursor and the engine.
	t.Helper()
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.ErrorLevel)
	cs := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	vol, err := volume_kvtx.NewVolume(ctx, "test/engine-scaling", store_kvkey.NewDefaultKVKey(), cs, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	bkt := volume_controller.NewBucketStore(vol, &bucket.Config{Id: "world", Rev: 1})
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, bkt, nil, &bucket.ObjectRef{BucketId: "world"}, &bucket.BucketOpArgs{BucketId: "world"}, nil)
	t.Cleanup(cursor.Release)
	authority := NewLocalAuthority(le, cursor, WorldRootName("scaling"), nil, coord.Scope{})
	engine, err := NewEngine(ctx, le, cursor.Clone(), world_mock.LookupMockOp, authority, false, WithReaderPinDelay(0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	// Write the n objects in batches of a hundred.
	for i := 0; i < n; i += 100 {
		w := fixtureWriter(t, engine)
		for j := i; j < min(i+100, n); j++ {
			fixtureObject(t, w, "object "+strconv.Itoa(j), "data "+strconv.Itoa(j))
		}
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Measure each entry point, counting the decoded block cache's hits.
	var hits atomic.Uint64
	block.SetDecodedBlockHitObserver(func(*block.BlockRef) { hits.Add(1) })
	t.Cleanup(func() { block.SetDecodedBlockHitObserver(nil) })
	costs := make(map[string]kvtx_kvtest.Counts)
	measure := func(name string, fn func()) {
		t.Helper()
		before, beforeHits := cs.Counts(), hits.Load()
		fn()
		c := cs.Counts().Sub(before)
		c.Reads += hits.Load() - beforeHits
		costs[name] = c
	}
	measure("read", func() {
		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		obj, found, err := tx.GetObject(ctx, "object 7")
		if err != nil || !found {
			t.Fatalf("read object 7: found %v err %v", found, err)
		}
		world.ReleaseObjectState(obj)
	})
	measure("commit", func() {
		w := fixtureWriter(t, engine)
		fixtureObject(t, w, "object 7", "changed")
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	})
	return costs
}

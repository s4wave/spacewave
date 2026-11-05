//go:build !js && !wasip1

package world_block

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
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
			locked := follower.bcast.Lock()
			changed := follower.headChanged
			locked.Unlock()
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
}

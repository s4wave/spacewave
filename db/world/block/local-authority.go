package world_block

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// WorldRootName returns the name of the root that holds the head of the World
// engine engineID in its bucket.
func WorldRootName(engineID string) string {
	return "world:" + engineID
}

// LocalAuthority is the Authority of a World whose head is a named root of
// the Volume that stores it. A write turn holds the Volume coordinator's lease
// on the root, so the writers of every process sharing the Volume take turns,
// and a candidate is accepted and durable at once when its compare and swap
// on the root lands, in the one Volume transaction that stores its blocks.
// A swap that finds another root supersedes the candidate.
//
// A turn keeps its base held from Begin until the swap, as the collector's
// invariant 7 requires of a writer that builds on a root it read: the base is
// the block the root holds, and the lease keeps every other writer off the
// root until the turn ends, so the root keeps holding it. A writer that lost
// the lease meanwhile is caught by the swap's Previous, and the collector's
// grace period covers its base until then.
type LocalAuthority struct {
	// le is the logger.
	le *logrus.Entry
	// root reads the World root of a head another writer committed.
	root *bucket_lookup.Cursor
	// store is the bucket store that holds the root.
	store block.StoreOps
	// name is the root's name.
	name string
	// coordinator orders writers across processes, or is nil when only this
	// process writes the Volume.
	coordinator coord.Coordinator
	// scope is the coordinator scope of the root.
	scope coord.Scope
	// turnMtx serializes turns when there is no coordinator.
	turnMtx csync.Mutex
	// bcast guards head and wakes the head watches when it changes.
	bcast broadcast.Broadcast
	// head is the newest head this authority read or decided.
	head world.Head
}

// NewLocalAuthority returns the authority of the World whose head is the root
// name in the bucket root reads.
// root is cloned, and its root reference is ignored.
// coordinator can be nil when no other process writes the Volume.
func NewLocalAuthority(
	le *logrus.Entry,
	root *bucket_lookup.Cursor,
	name string,
	coordinator coord.Coordinator,
	scope coord.Scope,
) *LocalAuthority {
	return &LocalAuthority{
		le:          le,
		root:        root.Clone(),
		store:       root.GetBucket(),
		name:        name,
		coordinator: coordinator,
		scope:       scope,
	}
}

// Head reads the current head: one indexed root read, plus one block read
// of the World root when the root moved since this authority last saw it.
func (a *LocalAuthority) Head(ctx context.Context) (world.Head, error) {
	// Read the root.
	ref, err := a.store.GetRoot(ctx, a.name)
	if err != nil {
		return world.Head{}, err
	}

	// Answer from the newest head when the root still holds it.
	var cached world.Head
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cached = a.head
	})
	if ref.GetEmpty() {
		return world.Head{}, nil
	}
	if ref.EqualsRef(cached.Root) {
		return cached, nil
	}

	// Read the seqno of the root another writer committed.
	seqno, err := a.readSeqno(ctx, ref)
	if err != nil {
		return world.Head{}, err
	}
	head := world.Head{Root: ref, Seqno: seqno, Durable: true}
	a.observe(head)
	return head, nil
}

// WatchHead calls fn with the current head, then with each head this process
// commits or the coordinator reports another process committed, until release.
// Each report costs one indexed root read; fn runs on one goroutine.
func (a *LocalAuthority) WatchHead(ctx context.Context, fn func(world.Head)) (func(), error) {
	// Subscribe to the coordinator before the first read, so a commit racing
	// the subscription is seen through either.
	ctx, cancel := context.WithCancel(ctx)
	var events <-chan coord.Event
	if a.coordinator != nil {
		watch, err := a.watchCoordinator(ctx)
		if err != nil {
			cancel()
			return nil, err
		}
		if watch != nil {
			events = watch.Events()
			context.AfterFunc(ctx, func() { _ = watch.Close() })
		}
	}

	// Deliver the head after each wake until release.
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.runHeadWatch(ctx, events, fn)
	}()
	return func() {
		cancel()
		<-done
	}, nil
}

// Begin waits for the coordinator's lease on the root, applies the commits
// other processes wrote, and reads the head the turn builds on.
func (a *LocalAuthority) Begin(ctx context.Context) (world.Turn, error) {
	// Take the turn.
	t := &localTurn{a: a}
	if a.coordinator != nil {
		lease, err := a.coordinator.WaitAcquireWriteLease(ctx, a.scope)
		if err != nil {
			return nil, err
		}
		t.lease = lease
		if _, err := lease.Refresh(ctx); err != nil && !errors.Is(err, coord.ErrUnsupported) {
			t.Release()
			return nil, err
		}
	} else {
		release, err := a.turnMtx.Lock(ctx)
		if err != nil {
			return nil, err
		}
		t.unlock = release
	}

	// Read the base under the turn.
	base, err := a.Head(ctx)
	if err != nil {
		t.Release()
		return nil, err
	}
	t.base = base
	return t, nil
}

// Install moves the root from previous to root under a turn, releasing the
// roots in release in the same transaction, and returns the new head. It
// opens a World: it creates its first root, adopts the head an older engine
// kept elsewhere, or replaces a head whose blocks are missing. root's blocks
// must be stored. Returns block.ErrRootMoved when the root does not hold
// previous.
func (a *LocalAuthority) Install(ctx context.Context, previous, root *block.BlockRef, release ...block.NamedRoot) (world.Head, error) {
	// Take a turn so no writer moves the root meanwhile.
	turn, err := a.Begin(ctx)
	if err != nil {
		return world.Head{}, err
	}
	defer turn.Release()

	// Read the new root's seqno, then swap it in with the releases.
	seqno, err := a.readSeqno(ctx, root)
	if err != nil {
		return world.Head{}, err
	}
	roots := append([]block.NamedRoot{{Name: a.name, Ref: root, Previous: previous}}, release...)
	if err := a.store.SetRoots(ctx, nil, roots); err != nil {
		return world.Head{}, err
	}
	head := world.Head{Root: root.Clone(), Seqno: seqno, Durable: true}
	turn.(*localTurn).published(ctx, head)
	return head, nil
}

// readSeqno reads the World seqno from the World root block at ref.
func (a *LocalAuthority) readSeqno(ctx context.Context, ref *block.BlockRef) (uint64, error) {
	// Follow the root in the World's bucket.
	cursor, err := a.root.FollowRef(ctx, &bucket.ObjectRef{RootRef: ref})
	if err != nil {
		return 0, err
	}
	defer cursor.Release()

	// Decode the World root and read its last change.
	_, bcs := cursor.BuildTransaction(nil)
	root, err := UnmarshalWorld(ctx, bcs)
	if err != nil {
		return 0, err
	}
	return root.GetLastChange().GetSeqno(), nil
}

// observe records head when it is past the newest head, and wakes the watches.
func (a *LocalAuthority) observe(head world.Head) {
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.head.Root.GetEmpty() || head.Seqno > a.head.Seqno || (head.Seqno == a.head.Seqno && !head.Root.EqualsRef(a.head.Root)) {
			a.head = head
			broadcast()
		}
	})
}

// watchCoordinator subscribes to the coordinator's events for the root, or
// returns nil when the coordinator reports none for it.
func (a *LocalAuthority) watchCoordinator(ctx context.Context) (coord.Watch, error) {
	// Start the watch at the current generation.
	snapshot, err := a.coordinator.Snapshot(ctx, a.scope)
	if errors.Is(err, coord.ErrUnsupported) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	watch, err := a.coordinator.Watch(ctx, a.scope, snapshot.Generation)
	if errors.Is(err, coord.ErrUnsupported) {
		return nil, nil
	}
	return watch, err
}

// runHeadWatch delivers the head to fn once, then again after each local
// commit or coordinator event that moved it, until ctx ends. It reads the root
// only for the first delivery and for a coordinator event that does not name
// the head this authority already holds: a local wake carries the head this
// authority just decided or read, and the event a local commit publishes names
// it, so a local commit costs the watch no read.
func (a *LocalAuthority) runHeadWatch(ctx context.Context, events <-chan coord.Event, fn func(world.Head)) {
	var last world.Head
	var delivered bool
	read := true
	for {
		// Capture the local wake before reading, so a commit after the read
		// wakes the next wait.
		var wake <-chan struct{}
		var cached world.Head
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			wake, cached = getWaitCh(), a.head
		})

		// Take the head, reading the root when the wake did not carry it, and
		// deliver it when it moved.
		head, err := cached, error(nil)
		if read {
			head, err = a.Head(ctx)
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.le.WithError(err).Warn("unable to read the World head")
		} else if !head.Root.GetEmpty() && (!delivered || !head.Root.EqualsRef(last.Root)) {
			fn(head)
			last, delivered = head, true
		}

		// Wait for a local commit or a coordinator event; read the root next
		// only for an event that may name another writer's head.
		select {
		case <-ctx.Done():
			return
		case <-wake:
			read = false
		case ev, ok := <-events:
			if !ok {
				events = nil
			}
			a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				cached = a.head
			})
			changed := ev.RootChanged.GetRootRef()
			read = !ok || changed.GetEmpty() || !changed.EqualsRef(cached.Root)
		}
	}
}

// _ is a type assertion
var _ world.Authority = (*LocalAuthority)(nil)

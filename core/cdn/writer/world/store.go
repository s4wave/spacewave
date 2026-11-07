// Package cdn_writer_world keeps CDN Space root pointers in a World: one
// CdnSpace object per Space, written by two typed operations, import and
// commit. The pointer state then survives restarts and replicates with the
// rest of the World. A server whose World runs these operations adds
// SpaceTypes to its schema.
package cdn_writer_world

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_writer "github.com/s4wave/spacewave/core/cdn/writer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// spacesPrefix keys each CdnSpace by its Space ID.
const spacesPrefix = "cdn/spaces/"

// SpaceKey is the World object key of the CdnSpace of spaceID.
func SpaceKey(spaceID string) string {
	return spacesPrefix + spaceID
}

// Store is a cdn_writer.Store over a World engine.
type Store struct {
	// eng is the World engine.
	eng world.Engine
	// ws applies each operation in its own write transaction.
	ws world.WorldState
	// sender is the peer the operations are applied as.
	sender peer.ID
}

// Store is a writer's store.
var _ cdn_writer.Store = (*Store)(nil)

// NewStore returns a store writing eng as sender.
func NewStore(eng world.Engine, sender peer.ID) *Store {
	return &Store{eng: eng, ws: world.NewEngineWorldState(eng, true), sender: sender}
}

// Root returns the recorded pointer of spaceID, or nil.
func (s *Store) Root(ctx context.Context, spaceID string) (*cdn.CdnRootPointer, error) {
	return lookupRoot(ctx, s.ws, spaceID)
}

// Import records root unless its Space is recorded, and returns the
// recorded pointer.
func (s *Store) Import(ctx context.Context, root *cdn.CdnRootPointer) (*cdn.CdnRootPointer, error) {
	if err := s.apply(ctx, &ImportCdnSpaceOp{Root: root}); err != nil {
		return nil, err
	}
	return s.Root(ctx, root.GetSpaceId())
}

// Commit applies c in one write transaction and returns the new pointer
// once it is durable: the one the operation wrote, with no second read.
func (s *Store) Commit(ctx context.Context, c *cdn_writer.Commit) (*cdn.CdnRootPointer, error) {
	op := &keptCommitOp{CommitCdnSpaceOp: &CommitCdnSpaceOp{
		SpaceId:       c.SpaceID,
		Checkpoint:    c.Checkpoint,
		Packs:         c.Packs,
		NowUnixMillis: c.Now.UnixMilli(),
		Signer:        c.Signer,
		Replaces:      c.Replaces,
		Retires:       c.Retires,
	}}
	if err := s.apply(ctx, op); err != nil {
		return nil, err
	}
	return op.root, nil
}

// keptCommitOp is a commit that keeps the pointer its last run wrote, since
// a retried transaction runs it again.
type keptCommitOp struct {
	*CommitCdnSpaceOp
	// root is the pointer the last run wrote or kept.
	root *cdn.CdnRootPointer
}

// ApplyWorldOp applies the commit and keeps the resulting pointer.
func (o *keptCommitOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	root, err := o.apply(ctx, ws)
	o.root = root
	return false, err
}

// apply applies op and waits until it is durable.
func (s *Store) apply(ctx context.Context, op world.Operation) error {
	seqno, _, err := s.ws.ApplyWorldOp(ctx, op, s.sender)
	if err != nil {
		return err
	}
	_, err = s.eng.SyncThrough(ctx, seqno)
	return err
}

// Validate checks the operation names a Space.
func (o *ImportCdnSpaceOp) Validate() error {
	if o.GetRoot().GetSpaceId() == "" {
		return errors.New("an imported root pointer needs a Space ID")
	}
	return nil
}

// ApplyWorldOp records the pointer when its Space is not yet recorded.
func (o *ImportCdnSpaceOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	// Keep a recorded Space as it is.
	if err := o.Validate(); err != nil {
		return false, err
	}
	key := SpaceKey(o.GetRoot().GetSpaceId())
	obj, exists, err := ws.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	if err != nil || exists {
		return false, err
	}

	// Create the typed object.
	created, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&CdnSpace{Root: o.GetRoot().CloneVT()}, true)
		return nil
	})
	world.ReleaseObjectState(created)
	if err != nil {
		return false, err
	}
	return false, world_types.SetObjectType(ctx, ws, key, CdnSpaceTypeID)
}

// Validate checks the operation names a Space.
func (o *CommitCdnSpaceOp) Validate() error {
	if o.GetSpaceId() == "" {
		return errors.New("a CDN Space commit needs a Space ID")
	}
	return nil
}

// ApplyWorldOp applies the commit to the recorded pointer with
// cdn_writer.ApplyCommit and writes it back when it changed.
func (o *CommitCdnSpaceOp) ApplyWorldOp(ctx context.Context, _ *logrus.Entry, ws world.WorldState, _ peer.ID) (bool, error) {
	_, err := o.apply(ctx, ws)
	return false, err
}

// apply applies the commit and returns the resulting pointer, which the
// caller must not change.
func (o *CommitCdnSpaceOp) apply(ctx context.Context, ws world.WorldState) (*cdn.CdnRootPointer, error) {
	// Require a recorded Space.
	if err := o.Validate(); err != nil {
		return nil, err
	}
	key := SpaceKey(o.GetSpaceId())
	obj, exists, err := ws.GetObject(ctx, key)
	if err != nil {
		world.ReleaseObjectState(obj)
		return nil, err
	}
	if !exists {
		return nil, cdn_writer.ErrSpaceUnknown
	}
	defer world.ReleaseObjectState(obj)

	// Apply the commit to a shallow copy of the object's pointer, writing it
	// only when it changed: ApplyCommit replaces fields and appends to a
	// clipped pack list, so the decoded entries are shared, not copied. The
	// callback derives everything from the cursor, since it runs again when
	// another writer changed the object first.
	c := &cdn_writer.Commit{
		SpaceID:    o.GetSpaceId(),
		Checkpoint: o.GetCheckpoint(),
		Packs:      o.GetPacks(),
		Now:        time.UnixMilli(o.GetNowUnixMillis()),
		Signer:     o.GetSigner(),
		Replaces:   o.GetReplaces(),
		Retires:    o.GetRetires(),
	}
	var out *cdn.CdnRootPointer
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		// Read the recorded pointer.
		space, err := block.UnmarshalBlock[CdnSpace](ctx, bcs)
		if err != nil {
			return err
		}
		src := space.GetRoot()
		if src == nil {
			return errors.Errorf("%s holds no root pointer", key)
		}

		// Apply the commit to a shallow copy.
		root := new(cdn.CdnRootPointer)
		*root = *src
		root.Packs = slices.Clip(root.Packs)
		changed, err := cdn_writer.ApplyCommit(root, c)
		if err != nil {
			return err
		}

		// Write the pointer back only when it changed.
		out = src
		if changed {
			out = root
			bcs.SetBlock(&CdnSpace{Root: root}, true)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// lookupRoot returns a copy of the recorded pointer of spaceID, or nil.
func lookupRoot(ctx context.Context, ws world.WorldState, spaceID string) (*cdn.CdnRootPointer, error) {
	space, err := world.LookupObjectBody[CdnSpace](ctx, ws, SpaceKey(spaceID))
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return space.GetRoot().CloneVT(), nil
}

// The operations are World operations.
var (
	_ world.Operation = (*ImportCdnSpaceOp)(nil)
	_ world.Operation = (*CommitCdnSpaceOp)(nil)
)

package space_world

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
)

// errRetainedRootNotHead is returned when the root to retain is no longer the
// installed World head.
var errRetainedRootNotHead = errors.New("root is not the current World head, export it again")

// Engine is the World engine of a Space: the one World engine over the
// Space's Adapter as its Authority. Writes run as this device's participant,
// and changes that are not World transactions, such as a retained root or the
// World's transform, are Space operations the Adapter queues.
type Engine struct {
	// Engine serves the Space World.
	*world_block.Engine
	// spaceBackfill serves the backfill methods of BackfillEngine.
	*spaceBackfill
	// a is the Space's adapter.
	a *Adapter
	// cursor is the engine's root cursor, released with it.
	cursor *bucket_lookup.Cursor
}

// GetAdapter returns the Space's adapter.
func (e *Engine) GetAdapter() *Adapter {
	return e.a
}

// OperationAuthor returns this device's peer and the person its participant
// acts as: its entity, or the device for a participant without one.
func (e *Engine) OperationAuthor(ctx context.Context) (peer.ID, string, error) {
	participant, err := e.a.participant(ctx)
	if err != nil {
		return "", "", err
	}
	return e.a.so.GetPeerID(), operationPerson(participant), nil
}

// NewTransaction opens a read snapshot, or a write that applies authenticated
// operations as this device's participant, as replay attributes them to the
// operation's signer.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &spaceWriteTx{Tx: tx, a: e.a}, nil
}

// NewRehearsalTransaction opens a write fork of the newest World that never
// commits, applying authenticated operations as NewTransaction applies them.
func (e *Engine) NewRehearsalTransaction(ctx context.Context) (world.Tx, error) {
	tx, err := e.Engine.NewRehearsalTransaction(ctx)
	if err != nil {
		return nil, err
	}
	return &spaceWriteTx{Tx: tx, a: e.a}, nil
}

// SetRetainedRoot retains the installed World head under name, or releases
// name when ref is empty, and waits for the operation's outcome.
func (e *Engine) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	// Refuse a malformed name, or a root other than the head.
	if err := validateRetainedRootName(name); err != nil {
		return err
	}
	head, err := e.a.Head(ctx)
	if err != nil {
		return err
	}
	if !ref.GetEmpty() && !ref.EqualsRef(head.Root) {
		return errRetainedRootNotHead
	}

	// Queue it and wait.
	return e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_SetRetainedRoot{SetRetainedRoot: &SetRetainedRootOp{Name: name, RootRef: ref}}})
}

// GetTransformConf returns the transform of the installed World head.
func (e *Engine) GetTransformConf(context.Context) (*block_transform.Config, error) {
	var conf *block_transform.Config
	e.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		conf = e.a.headState.GetHeadRef().GetTransformConf().CloneVT()
	})
	return conf, nil
}

// SetTransformConf changes the World transform to conf and waits for the
// operation's outcome. conf must read every block the current transform
// wrote. Setting the current transform does nothing.
func (e *Engine) SetTransformConf(ctx context.Context, conf *block_transform.Config) error {
	// Skip the current transform and refuse one replay would reject.
	prev, err := e.GetTransformConf(ctx)
	if err != nil {
		return err
	}
	if prev.EqualVT(conf) {
		return nil
	}
	if err := validateTransformSuccessor(prev, conf); err != nil {
		return err
	}

	// Queue it and wait.
	return e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_SetTransformConf{SetTransformConf: &SetTransformConfOp{TransformConf: conf}}})
}

// StepGraphSettings reports whether the head World has the target graph
// settings, and if not queues one GraphSettingsStepOp and waits for its
// outcome.
func (e *Engine) StepGraphSettings(ctx context.Context, target *world_block.GraphSettings, max int) (bool, error) {
	// Refuse a step replay would reject.
	if err := validateGraphSettingsStep(target, max); err != nil {
		return false, err
	}
	want, err := normalizeGraphSettings(target)
	if err != nil {
		return false, err
	}

	// Report a World that already has the target settings.
	rtx, err := e.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	have, err := rtx.GetGraphSettings(ctx)
	rtx.Discard()
	if err != nil {
		return false, err
	}
	if have.EqualVT(want) {
		return true, nil
	}

	// Queue one step and wait.
	return false, e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_GraphSettingsStep{
		GraphSettingsStep: &GraphSettingsStepOp{Target: target, Max: uint32(max)}, //nolint:gosec // validateGraphSettingsStep bounds max.
	}})
}

// RegenerateWorld moves the World to target through the Space's order, as
// world_block.WorldRegenerator says, and waits for the outcome. To the node
// graph it orders a pin, which adds the quads of the quad list at quads to
// the kv World, waits until this device built the pin off the replay path,
// and orders the switch, which every replica, this one included, applies by
// carrying its build of the pin forward to the head; the writers wait only
// for that carry. A refusal returns sobject.ErrRejectedOp.
// The Space holds the list from before the operation is queued until its
// checkpoint passes the operation, as it holds a transaction's payloads, so
// the caller must hold the list, as an open stage does, until RegenerateWorld
// returns; a list nothing holds is refused with block.ErrRootNotHeld.
func (e *Engine) RegenerateWorld(ctx context.Context, target world_block.WorldFormat, quads *block.BlockRef) error {
	// A switch back is one operation.
	if target != world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		return e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_RegenerateWorld{RegenerateWorld: &RegenerateWorldOp{Target: target}}})
	}

	// Pin the World with the carried quads, wait for this device's build of
	// the pin, and switch to it. A head that moved too far, or a pin another
	// device moved, pins again with no quads, since the first pin committed
	// them.
	for try := 0; ; try++ {
		var payloads []*block.BlockRef
		if !quads.GetEmpty() {
			payloads = []*block.BlockRef{quads}
		}
		pinOp := &RegenerateWorldOp{Target: target, Quads: quads}
		if err := e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_RegenerateWorld{RegenerateWorld: pinOp}}, payloads...); err != nil {
			return err
		}
		quads = nil
		res, err := e.a.regenBuild.waitPinned(ctx)
		if err != nil {
			return err
		}
		if res == nil {
			return errors.Wrap(sobject.ErrRejectedOp, "world regeneration: the pin was switched or released")
		}

		// Release a pin no replica can build.
		if res.invalid != "" {
			release := &ReleaseWorldRegenOp{Base: res.pin}
			if err := e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_ReleaseWorldRegen{ReleaseWorldRegen: release}}); err != nil {
				return err
			}
			return errors.Wrap(sobject.ErrRejectedOp, "world regeneration failed: "+res.invalid)
		}

		// Switch, pinning again when the head moved or the pin did.
		switchOp := &RegenerateWorldOp{Target: target, Base: res.pin}
		err = e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_RegenerateWorld{RegenerateWorld: switchOp}})
		if err == nil || try >= maxRegenPins-1 || !errors.Is(err, sobject.ErrRejectedOp) {
			return err
		}
		if msg := err.Error(); !strings.Contains(msg, reasonRegenMoved) && !strings.Contains(msg, reasonRegenPinDiffers) {
			return err
		}
	}
}

// maxRegenPins bounds the pins RegenerateWorld writes before it returns the
// switch's refusal.
const maxRegenPins = 3

// CompareWorldRegen compares the old World a regeneration holds with the new
// one on this device, off replay, and releases nothing.
func (e *Engine) CompareWorldRegen(ctx context.Context) error {
	_, _, err := e.compareRegen(ctx)
	return err
}

// ReleaseWorldRegen compares the old World a regeneration holds with the new
// one on this device, off replay, and when they are equal releases both
// through the Space's order. The release names the roots compared, so replay
// refuses it if another regeneration holds others by then.
func (e *Engine) ReleaseWorldRegen(ctx context.Context) error {
	base, built, err := e.compareRegen(ctx)
	if err != nil {
		return err
	}
	return e.a.writeOp(ctx, &WorldOp{Body: &WorldOp_ReleaseWorldRegen{ReleaseWorldRegen: &ReleaseWorldRegenOp{Base: base, Built: built}}})
}

// compareRegen compares the roots a regeneration holds, one comparison at a
// time, and returns them when they hold the same World.
func (e *Engine) compareRegen(ctx context.Context) (base, built *block.BlockRef, err error) {
	// Read the held roots.
	e.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, base, built = heldRegen(e.a.headState)
	})
	if base == nil || built == nil {
		return nil, nil, errors.New("no world regeneration is held")
	}

	// Compare them under the comparison lock.
	unlock, err := e.a.regenCompare.Lock(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = world_block.NewOrderedRegen(e.cursor, nil, e.a.regenConfig(regenSpillCompare)).Compare(ctx, base, built)
	unlock()
	return base, built, err
}

// GetHead returns the installed World root and its seqno.
func (e *Engine) GetHead(ctx context.Context) (*block.BlockRef, uint64, error) {
	head, err := e.a.Head(ctx)
	return head.Root, head.Seqno, err
}

// retire retires the engine for the next adapter of the Space and releases
// its cursor: the transactions open on it end with world.ErrEngineRetired.
func (e *Engine) retire() error {
	err := e.Retire()
	e.cursor.Release()
	return err
}

// spaceWriteTx is a Space write transaction: an authenticated operation runs
// as this device with its participant's person and role, as replay runs it.
type spaceWriteTx struct {
	// Tx is the engine's write transaction.
	world.Tx
	// a is the Space's adapter.
	a *Adapter
}

// ApplyWorldOp applies op, as this device for an authenticated operation.
func (t *spaceWriteTx) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	if _, authenticated := op.(world.AuthenticatedOperation); authenticated {
		participant, err := t.a.participant(ctx)
		if err != nil {
			return 0, false, err
		}
		sender = t.a.so.GetPeerID()
		ctx = withOperationAuthor(ctx, participant)
	}
	return t.Tx.ApplyWorldOp(ctx, op, sender)
}

// WaitObjectRev waits through the engine's write transaction.
func (t *spaceWriteTx) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return world.WaitObjectRev(ctx, t.Tx, key, rev, ignoreNotFound)
}

// participant returns this device's participant in the accepted config.
func (a *Adapter) participant(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	return snap.GetParticipantConfigForPeer(ctx, a.so.GetPeerID().String())
}

// writeOp queues op after this device's admitted operations and waits for
// replay to decide it. Like Admit, it waits while replay opens and returns
// replay's wait while replay waits, since its caller chose op from the
// installed head. payloads are the roots op names, which writeOp holds before
// queueing op, refusing one nothing holds now with block.ErrRootNotHeld.
func (a *Adapter) writeOp(ctx context.Context, op *WorldOp, payloads ...*block.BlockRef) error {
	// Queue it unless replay is not caught up, holding its payloads first.
	data, err := op.MarshalVT()
	if err != nil {
		return err
	}
	adm := a.newAdmission(world.Candidate{}, data, payloads, nil)
	if err := a.admitWrite(ctx, adm); err != nil {
		return err
	}
	if err := a.holdAdmitted(ctx, adm, false); err != nil {
		a.fail(adm, err)
		return err
	}
	a.queue.push(adm)

	// Wait for its decision.
	decision, err := adm.WaitDecided(ctx)
	switch {
	case err != nil:
		return err
	case decision != world.Accepted:
		return errors.Wrap(sobject.ErrRejectedOp, adm.reason)
	}
	return nil
}

// normalizeGraphSettings returns the settings a World root stores for target:
// nil for the default settings.
func normalizeGraphSettings(target *world_block.GraphSettings) (*world_block.GraphSettings, error) {
	if len(target.GetIndexes()) == 0 {
		return nil, nil
	}
	indexes, err := target.QuadIndexes()
	if err != nil {
		return nil, err
	}
	return world_block.NewGraphSettings(indexes)
}

// _ is a type assertion
var (
	_ world.Engine                     = (*Engine)(nil)
	_ world_block.GraphSettingsStepper = (*Engine)(nil)
	_ world.RehearsalEngine            = (*Engine)(nil)
	_ BackfillEngine                   = (*Engine)(nil)
	_ worldReaders                     = (*Engine)(nil)
)

// _ is a type assertion
var _ world_block.WorldRegenerator = (*Engine)(nil)

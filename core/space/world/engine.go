package space_world

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
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
	// Read this device's participant.
	participant, err := e.a.participant(ctx)
	if err != nil {
		return "", "", err
	}

	// Name its person.
	person := participant.GetEntityId()
	if person == "" {
		person = participant.GetPeerId()
	}
	return e.a.so.GetPeerID(), person, nil
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

// GetBackfill watches this device's backfill choice.
func (e *Engine) GetBackfill() ccontainer.Watchable[bool] {
	return e.a.backfill.choice
}

// GetBackfillState watches the outcome of the last finished copy.
func (e *Engine) GetBackfillState() ccontainer.Watchable[*BackfillState] {
	return e.a.backfill.state
}

// GetHead returns the installed World root and its seqno.
func (e *Engine) GetHead(ctx context.Context) (*block.BlockRef, uint64, error) {
	head, err := e.a.Head(ctx)
	return head.Root, head.Seqno, err
}

// SetBackfill stores this device's backfill choice and starts or stops the
// copy.
func (e *Engine) SetBackfill(ctx context.Context, enabled bool) error {
	return e.a.backfill.set(ctx, enabled)
}

// RecheckBackfill restarts the copy as a recheck.
func (e *Engine) RecheckBackfill(context.Context) error {
	return e.a.backfill.recheckNow()
}

// close closes the engine and releases its cursor.
func (e *Engine) close() error {
	err := e.Close()
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

// participant returns this device's participant in the accepted config.
func (a *Adapter) participant(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	return snap.GetParticipantConfigForPeer(ctx, a.so.GetPeerID().String())
}

// writeOp queues op after this device's admitted operations and waits for
// replay to decide it.
func (a *Adapter) writeOp(ctx context.Context, op *WorldOp) error {
	// Encode and queue it.
	data, err := op.MarshalVT()
	if err != nil {
		return err
	}
	adm := a.newAdmission(world.Candidate{}, data, nil)
	a.admit(adm)
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
	_ BackfillEngine                   = (*Engine)(nil)
)

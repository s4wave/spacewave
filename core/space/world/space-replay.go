package space_world

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
)

// maxRetainedRoots is the most retained roots a World holds. The roots live in
// the checkpoint's state data, which has a small size limit.
const maxRetainedRoots = 16

// maxRetainedRootNameLen is the longest retained root name in bytes.
const maxRetainedRootNameLen = 64

// maxGraphSettingsStep bounds GraphSettingsStepOp.max, keeping one replayed
// step's commit short for every member.
const maxGraphSettingsStep = 65536

// spacePosition is one operation applied above the installed checkpoint and
// the World after it.
type spacePosition struct {
	// op is the operation.
	op *sobject.OrderedOperation
	// reason is why replay refused the operation, or empty when it applied.
	reason string
	// state is the World after the operation.
	state *InnerState
	// seqno is the World seqno at state.
	seqno uint64
	// payloads are the roots of the operation's payload, which replay reads
	// again while the operation is above the checkpoint.
	payloads []*block.BlockRef
}

// spaceReplay computes the Worlds of the positions above the installed
// checkpoint on a replay engine: a db/world/block Engine with no Authority
// over the adapter's PendingStore, through the replay stage on a serving
// adapter, bound to the World before the position it applies, whose commits
// stay in memory until they outgrow it. It keeps the World after every
// position, so a reorder resumes after the longest prefix the new order
// shares. The driver's goroutine owns it.
type spaceReplay struct {
	// a is the adapter.
	a *Adapter
	// base is the World of the installed checkpoint.
	base *InnerState
	// baseSeqno is the World seqno at base.
	baseSeqno uint64
	// height is the height of the installed checkpoint.
	height uint64
	// positions are the applied operations above the checkpoint, in order.
	positions []spacePosition
	// saved is how many positions the saved cursor records.
	saved int
	// floor is how many leading positions may have Worlds whose pages left
	// memory unstored, or lost the epoch that held them. Replay computes them
	// again from base before it runs an operation on one of them.
	floor int
	// restored are the positions a saved cursor recorded, which an Apply of
	// the same operation at the same place adopts.
	restored []spacePosition
	// restoredFrom is the World the restored positions start from.
	restoredFrom *InnerState
	// engine is the replay engine, bound to bound, or nil.
	engine *world_block.Engine
	// cursor is the engine's root cursor.
	cursor *bucket_lookup.Cursor
	// bound is the World engine serves.
	bound *bucket.ObjectRef
	// stage holds what a serving adapter's replay stores until a cursor save
	// lands, or is nil for a transient replay, whose kept pages already go
	// to a stage the caller holds. Its methods do nothing on nil.
	stage *replayStage
}

// store returns the store the replay writes through.
func (r *spaceReplay) store() block.StoreOps {
	if r.stage == nil {
		return r.a.pages
	}
	return r.stage
}

// stateBefore returns the World before the position at index and its seqno.
func (r *spaceReplay) stateBefore(index int) (*InnerState, uint64) {
	if index == 0 {
		return r.base, r.baseSeqno
	}
	pos := r.positions[index-1]
	return pos.state, pos.seqno
}

// head returns the World after the last position and its seqno.
func (r *spaceReplay) head() (*InnerState, uint64) {
	return r.stateBefore(len(r.positions))
}

// restoredAt returns the saved position at index when it records op after the
// same World, or nil.
func (r *spaceReplay) restoredAt(index int, op *sobject.OrderedOperation, prev *InnerState) *spacePosition {
	// The saved position must record op.
	if index >= len(r.restored) || !bytes.Equal(r.restored[index].op.Hash, op.Hash) {
		return nil
	}

	// It must follow the same World.
	before := r.restoredFrom
	if index != 0 {
		before = r.restored[index-1].state
	}
	if !before.EqualVT(prev) {
		return nil
	}
	pos := r.restored[index]
	pos.op = op
	return &pos
}

// run computes the position of op after the World before index. adopt, when
// set, is the World a local writer built on that World with op, which the
// position takes without running op again.
func (r *spaceReplay) run(ctx context.Context, index int, op *sobject.OrderedOperation, adopt *spacePosition) (spacePosition, error) {
	// The World before index may have left memory unstored. Only running an
	// operation reads its pages; the other outcomes compare and copy its
	// InnerState, which computing it again leaves the same.
	unstored := index != 0 && index <= r.floor
	prev, prevSeqno := r.stateBefore(index)
	pos := spacePosition{op: op, state: prev, seqno: prevSeqno, reason: op.Refused}

	// An operation every member refuses, or an acknowledgment, leaves the
	// World, so its position shares the floor of the World before it.
	if op.Refused != "" || op.Data == nil {
		if unstored {
			r.floor = max(r.floor, index+1)
		}
		return pos, nil
	}

	// Take a local writer's World, or a saved position, built on this World.
	if adopt != nil {
		adopt.op = op
		return *adopt, nil
	}
	if saved := r.restoredAt(index, op, prev); saved != nil {
		// Only the last saved World is stored: the ones before it are
		// computed again before an operation runs on them.
		last := len(r.restored) - 1
		r.floor = max(r.floor, index+1)
		if index == last {
			r.floor = max(r.floor-1, index)
		}
		return *saved, nil
	}

	// Compute the World before index again when it left memory unstored, then
	// run the operation on it.
	if unstored {
		if err := r.recompute(ctx, index); err != nil {
			return spacePosition{}, err
		}
		pos.state, pos.seqno = r.stateBefore(index)
	}
	ctx, task := trace.NewTask(ctx, "space/world/replay/apply")
	defer task.End()
	return r.apply(ctx, pos, op)
}

// recompute runs the positions before index again from base, after their
// Worlds left memory unstored.
func (r *spaceReplay) recompute(ctx context.Context, index int) error {
	for i := range index {
		state, seqno := r.stateBefore(i)
		pos, err := r.apply(ctx, spacePosition{op: r.positions[i].op, state: state, seqno: seqno}, r.positions[i].op)
		if err != nil {
			return err
		}
		r.positions[i] = pos
	}
	r.floor = 0
	return nil
}

// apply runs op on the World of pos and returns the position after it.
func (r *spaceReplay) apply(ctx context.Context, pos spacePosition, op *sobject.OrderedOperation) (spacePosition, error) {
	// Refuse an acknowledgment's absence of data or a refusal outright.
	pos.op, pos.reason = op, op.Refused
	if op.Refused != "" || op.Data == nil {
		return pos, nil
	}

	// Decode the operation.
	wop := &WorldOp{}
	if err := wop.UnmarshalVT(op.Data); err != nil {
		pos.reason = "invalid operation data: " + err.Error()
		return pos, nil
	}
	peerID, err := op.Inner.ParsePeerID()
	if err != nil {
		return pos, err
	}
	ctx = withOperationAuthor(ctx, op.Author)

	// Apply it by type.
	prev := pos.state
	var next *InnerState
	var reason string
	switch body := wop.GetBody().(type) {
	case *WorldOp_InitWorld:
		next, reason, err = r.applyInit(ctx, body.InitWorld, prev)
	case *WorldOp_ApplyTxOp:
		next, reason, err = r.applyTx(ctx, body.ApplyTxOp, prev, peerID)
		if err == nil && reason == "" {
			pos.payloads, err = body.ApplyTxOp.GetTx().PayloadRefs(ctx, r.a.lookupOp)
		}
	case *WorldOp_SetRetainedRoot:
		next, reason = applySetRetainedRoot(body.SetRetainedRoot, prev)
	case *WorldOp_SetTransformConf:
		next, reason = applySetTransformConf(body.SetTransformConf, prev)
	case *WorldOp_GraphSettingsStep:
		next, reason, err = r.applyGraphSettingsStep(ctx, body.GraphSettingsStep, prev)
	case *WorldOp_Fence, *WorldOp_RootSet:
		next = prev
	default:
		reason = "unknown operation type"
	}
	if err != nil {
		return pos, err
	}

	// Record the outcome, and the seqno of a changed World.
	if reason != "" {
		pos.reason = reason
		return pos, nil
	}
	pos.state = next
	if !next.GetHeadRef().EqualVT(prev.GetHeadRef()) {
		if pos.seqno, err = r.seqno(ctx, next); err != nil {
			return pos, err
		}
	}
	return pos, nil
}

// applyInit initializes an empty World.
func (r *spaceReplay) applyInit(ctx context.Context, initOp *InitWorldOp, prev *InnerState) (*InnerState, string, error) {
	// Refuse to initialize a World twice, or with a transform that does not
	// authenticate.
	if !prev.GetHeadRef().GetEmpty() {
		return nil, "world is already initialized", nil
	}
	next, err := BuildInitialInnerState(initOp)
	if err != nil {
		return nil, "invalid world init: " + err.Error(), nil
	}

	// Write the root of a World that starts without a changelog or with graph
	// settings.
	settings := initOp.GetGraphSettings()
	if !initOp.GetLastChangeDisable() && len(settings.GetIndexes()) == 0 {
		return next, "", nil
	}

	// Build the transformer of the World's transform.
	xfrm, err := newWorldTransformer(controller.ConstructOpts{Logger: r.a.le}, r.a.opts.StepFactorySet, next.GetHeadRef().GetTransformConf())
	if err != nil {
		return nil, "", err
	}

	// Write it to the kept pages.
	btx, bcs := block.NewTransaction(r.store(), xfrm, nil, nil)
	root := world_block.NewWorld(initOp.GetLastChangeDisable())
	root.GraphSettings = settings.CloneVT()
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		return nil, "", err
	}
	next.HeadRef.RootRef = rootRef
	return next, "", nil
}

// applyTx runs a World transaction as its signer on the World prev.
func (r *spaceReplay) applyTx(ctx context.Context, txOp *ApplyTxOp, prev *InnerState, peerID peer.ID) (*InnerState, string, error) {
	return r.commitOn(ctx, prev, "transaction apply failed", func(btx *world_block.EngineTx) (bool, error) {
		// Decode the transaction and run it as its signer.
		ttx, err := txOp.GetTx().LocateTx()
		if err != nil {
			return false, err
		}
		_, err = ttx.ExecuteTx(ctx, peerID, declaredLookup(r.a.lookupOp, r.a.opts.OpTypes.Declared, btx), btx)
		return err == nil, err
	})
}

// applyGraphSettingsStep moves the World prev one step toward the step's
// target graph settings.
func (r *spaceReplay) applyGraphSettingsStep(ctx context.Context, stepOp *GraphSettingsStepOp, prev *InnerState) (*InnerState, string, error) {
	// Refuse a step on no World or out of bounds.
	if prev.GetHeadRef().GetEmpty() {
		return nil, "world is not initialized", nil
	}
	if err := validateGraphSettingsStep(stepOp.GetTarget(), int(stepOp.GetMax())); err != nil {
		return nil, err.Error(), nil
	}

	// Step the World.
	return r.commitOn(ctx, prev, "graph settings step failed", func(btx *world_block.EngineTx) (bool, error) {
		_, changed, err := btx.StepGraphSettings(ctx, stepOp.GetTarget(), int(stepOp.GetMax()))
		return changed, err
	})
}

// commitOn runs change on a write transaction of the replay engine at the
// World prev and commits it when change reports a change. It returns the World
// after the commit, or prev when nothing changed. A failure of change refuses
// the operation with a reason that starts with failed, but a canceled read, a
// missing block or a pending handler stops replay instead: none is a property
// of the operation, so refusing it would split replicas.
func (r *spaceReplay) commitOn(ctx context.Context, prev *InnerState, failed string, change func(*world_block.EngineTx) (bool, error)) (*InnerState, string, error) {
	// A change needs a World.
	if prev.GetHeadRef().GetEmpty() {
		return nil, "world is not initialized", nil
	}

	// Run the change on the World and commit it.
	var next *bucket.ObjectRef
	aerr := func() error {
		// Open a writer on the World.
		btx, err := r.writer(ctx, prev.GetHeadRef())
		if err != nil {
			return err
		}
		defer btx.Discard()

		// Change it, and commit a change.
		changed, err := change(btx)
		if err != nil || !changed {
			return err
		}
		next, err = btx.CommitBlockTransaction(ctx)
		return err
	}()

	// Stop for a gap, refuse a failed change, and keep an unchanged World.
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if isReplayGap(aerr) {
		return nil, "", aerr
	}
	if aerr != nil {
		return nil, failed + ": " + aerr.Error(), nil
	}
	if next == nil {
		return prev, "", nil
	}
	return r.advance(prev, next), "", nil
}

// advance returns prev with its head at next, without a bucket ID.
func (r *spaceReplay) advance(prev *InnerState, next *bucket.ObjectRef) *InnerState {
	out := prev.CloneVT()
	out.HeadRef = next.CloneVT()
	out.HeadRef.BucketId = ""
	return out
}

// writer opens a write transaction of the replay engine at head.
func (r *spaceReplay) writer(ctx context.Context, head *bucket.ObjectRef) (*world_block.EngineTx, error) {
	engine, err := r.bind(ctx, head)
	if err != nil {
		return nil, err
	}
	return engine.NewBlockEngineTransaction(ctx, true)
}

// seqno returns the World seqno at state.
func (r *spaceReplay) seqno(ctx context.Context, state *InnerState) (uint64, error) {
	if state.GetHeadRef().GetRootRef().GetEmpty() {
		return 0, nil
	}
	engine, err := r.bind(ctx, state.GetHeadRef())
	if err != nil {
		return 0, err
	}
	return engine.GetSeqno(ctx)
}

// bind returns the replay engine at head, rebuilding it when it serves another
// World than head.
func (r *spaceReplay) bind(ctx context.Context, head *bucket.ObjectRef) (*world_block.Engine, error) {
	// Reuse the engine at head.
	if r.engine != nil {
		if r.engine.GetRootRef().EqualsRef(r.a.bucketRef(head)) {
			return r.engine, nil
		}
		r.close()
	}

	// Build an engine at head whose commits stay in memory.
	cursor, err := r.a.newCursor(ctx, r.store(), head)
	if err != nil {
		return nil, err
	}
	engine, err := world_block.NewEngine(ctx, r.a.le, cursor, r.a.lookupOp, nil, false, world_block.WithDeferredDurability())
	if err != nil {
		cursor.Release()
		return nil, err
	}
	r.engine, r.cursor, r.bound = engine, cursor, head.CloneVT()
	return engine, nil
}

// close releases the replay engine.
func (r *spaceReplay) close() {
	if r.engine == nil {
		return
	}
	_ = r.engine.Close()
	r.cursor.Release()
	r.engine, r.cursor, r.bound = nil, nil, nil
}

// payloads returns the payload roots of the positions above the checkpoint.
func (r *spaceReplay) payloads() []*block.BlockRef {
	var out []*block.BlockRef
	for _, pos := range r.positions {
		out = append(out, pos.payloads...)
	}
	return out
}

// applySetRetainedRoot sets or releases one retained root of prev.
func applySetRetainedRoot(setOp *SetRetainedRootOp, prev *InnerState) (*InnerState, string) {
	// Check the name.
	if err := validateRetainedRootName(setOp.GetName()); err != nil {
		return nil, err.Error()
	}

	// Replace, add, or remove the entry, keeping the list sorted by name.
	roots := slices.Clone(prev.GetRetainedRoots())
	idx, found := slices.BinarySearchFunc(roots, setOp.GetName(), func(root *RetainedRoot, name string) int {
		return strings.Compare(root.GetName(), name)
	})
	switch {
	case setOp.GetRootRef().GetEmpty() && found:
		roots = slices.Delete(roots, idx, idx+1)
	case setOp.GetRootRef().GetEmpty():
	case found:
		roots[idx] = &RetainedRoot{Name: setOp.GetName(), RootRef: setOp.GetRootRef()}
	case len(roots) >= maxRetainedRoots:
		return nil, "too many retained roots"
	default:
		roots = slices.Insert(roots, idx, &RetainedRoot{Name: setOp.GetName(), RootRef: setOp.GetRootRef()})
	}

	// Accept it.
	next := prev.CloneVT()
	next.RetainedRoots = roots
	return next, ""
}

// applySetTransformConf changes the transform of the World prev.
func applySetTransformConf(setOp *SetTransformConfOp, prev *InnerState) (*InnerState, string) {
	// The World must exist and the new transform must read its blocks.
	if prev.GetHeadRef().GetEmpty() {
		return nil, "world is not initialized"
	}
	if err := validateTransformSuccessor(prev.GetHeadRef().GetTransformConf(), setOp.GetTransformConf()); err != nil {
		return nil, err.Error()
	}

	// Accept it.
	next := prev.CloneVT()
	next.HeadRef.TransformConf = setOp.GetTransformConf().CloneVT()
	return next, ""
}

// validateRetainedRootName checks a retained root name.
func validateRetainedRootName(name string) error {
	if name == "" || len(name) > maxRetainedRootNameLen {
		return errors.Errorf("retained root name must be 1 to %d bytes", maxRetainedRootNameLen)
	}
	return nil
}

// validateGraphSettingsStep checks a step's target and bound.
func validateGraphSettingsStep(target *world_block.GraphSettings, max int) error {
	if max < 1 || max > maxGraphSettingsStep {
		return errors.Errorf("graph settings step max must be 1 to %d", maxGraphSettingsStep)
	}
	return target.ValidateInitial()
}

// withOperationAuthor binds the person a participant's operation runs as and
// whether the participant owns the Space.
func withOperationAuthor(ctx context.Context, participant *sobject.SOParticipantConfig) context.Context {
	// An operation without an author runs as no one.
	if participant == nil {
		return ctx
	}
	ctx = world.WithOperationPerson(ctx, operationPerson(participant))
	return world.WithOperationOwner(ctx, sobject.IsOwner(participant.GetRole()))
}

// operationPerson returns the person a participant's operations run as: its
// entity, or the device for a participant without one.
func operationPerson(participant *sobject.SOParticipantConfig) string {
	if person := participant.GetEntityId(); person != "" {
		return person
	}
	return participant.GetPeerId()
}

// newTransformer builds the transformer of a World transform.
func (a *Adapter) newTransformer(conf *block_transform.Config) (*block_transform.Transformer, error) {
	return newWorldTransformer(controller.ConstructOpts{Logger: a.le}, a.opts.StepFactorySet, conf)
}

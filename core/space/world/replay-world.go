package space_world

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/sirupsen/logrus"
)

// ReplayWorld replays the operation set of snap as the Space World engine
// engineID, and returns the World after it. The pages it computes land in a
// stage on so's block store, which holds them until release, so the caller
// copies the World before it releases.
func ReplayWorld(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
) (*InnerState, func(), error) {
	a, release, err := replayOnce(ctx, le, b, sfs, so, engineID, opTypes, snap, 0)
	if err != nil {
		return nil, nil, err
	}
	state, _ := a.replay.head()
	return state, release, nil
}

// replayOnce replays snap on a transient adapter whose pages stay in memory
// up to maxPending bytes and go to a stage on so's block store past it. The
// stage holds them until release. It waits for the trims its installs
// start.
func replayOnce(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
	maxPending int,
) (*Adapter, func(), error) {
	// Hold what the replay writes under a stage.
	staged, release, err := block.OpenStage(ctx, so.GetBlockStore())
	if err != nil {
		return nil, nil, err
	}

	// Replay through a transient adapter over it.
	a, err := NewAdapter(le, so, AdapterOptions{
		Bus:             b,
		StepFactorySet:  sfs,
		EngineID:        engineID,
		OpTypes:         opTypes,
		MaxPendingBytes: maxPending,
		transient:       true,
		pagesStore:      staged,
	})
	if err != nil {
		release()
		return nil, nil, err
	}
	defer a.replay.close()
	err = sobject.NewBodyDriver(le, so, a, nil).Step(ctx, snap)
	a.waitBackground()
	if err != nil {
		release()
		return nil, nil, err
	}
	return a, release, nil
}

// ErrUnresolvedOp is returned by ImportRoots when the type of an operation
// above the checkpoint does not resolve, such as a plugin's before the plugin
// registers, so its payloads cannot all be named.
var ErrUnresolvedOp = errors.New("an operation's type does not resolve")

// ImportRoots returns the roots a replay of snap, the state of so, reads, so a
// copy of them lets another device replay the state without this one: the
// World its checkpoint carries, if any, then the payloads of the operations
// above it. It resolves operation types as the Adapter of so for the Space
// World engine engineID on b does, with opTypes, looking each type up on b
// once.
//
// An operation whose type resolves nowhere names its payloads only in part, so
// ImportRoots names that part where the source's replay refuses it and fails
// with ErrUnresolvedOp otherwise. Replay refuses it when the Adapter's saved
// replay cursor records it as refused at the same position above the same
// checkpoint, and decides one the cursor has not reached at the World before
// it: the World after the cursor's last matching outcome, or the checkpoint's
// World without one, kept across the operations so decided, which leave it.
// There replay refuses an operation whose transaction runs it first, before
// anything changes that World, when no plugin the World installs declares
// its type (OpTypes.Declared). A refused operation still names every payload
// it can, at the cost of bytes the target may not read. Invariant: an
// operation gets no payloads only when every replay of this state would refuse
// it whatever its handlers, or its payloads cannot be named.
func ImportRoots(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	engineID string,
	opTypes OpTypes,
	snap sobject.SharedObjectStateSnapshot,
) ([]*block.BlockRef, error) {
	// Resolve the types as the Adapter does, one bus lookup per type.
	lookupOp, release := opTypes.HeldLookupOn(b, le, engineID)
	defer release()

	// Read the checkpoint's World and the operations above it.
	base, roots, err := checkpointWorld(ctx, snap)
	if err != nil {
		return nil, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}

	// Read the outcomes the source's replay saved above the same checkpoint.
	var saved []*ReplayCursorOutcome
	var states []*InnerState
	cursor, err := readSavedCursor(ctx, so)
	if err != nil {
		return nil, err
	}
	if cursor.GetBase().EqualVT(base) {
		var ok bool
		if states, ok = cursor.states(); ok {
			saved = cursor.GetOutcomes()
		}
	}

	// Name each operation's payloads, following the saved outcomes while they
	// record the same operations. prev is the World before the operation
	// while it is known; declared holds each World's declarations read so far.
	prev := base
	declared := make(map[declaredKey][]string)
	for i, h := range set.Order() {
		// Take a saved outcome. A refusal may only mean the source lacked a
		// handler another device has, so a refused operation still names
		// its payloads where it can.
		if i < len(saved) && bytes.Equal(saved[i].GetHash(), h) {
			prev = states[i]
			refs, known, err := opPayloads(ctx, snap, set, h, lookupOp)
			if err != nil {
				return nil, err
			}
			roots = append(roots, refs...)
			if !known && saved[i].GetReason() == "" {
				return nil, ErrUnresolvedOp
			}
			continue
		}
		saved = nil

		// Name the payloads of an operation replay has not reached. One that
		// runs no transaction names none; the World after one that changes it
		// another way is not known here.
		tx, leaves, err := replayedTx(ctx, snap, set, h)
		if err != nil {
			return nil, err
		}
		if tx == nil {
			if !leaves {
				prev = nil
			}
			continue
		}
		refs, err := tx.PayloadRefs(ctx, lookupOp)
		roots = append(roots, refs...)
		if err == nil {
			prev = nil
			continue
		}

		// Decide whether replay refuses one whose type does not resolve, at
		// the World before it.
		refused, err := refusedAt(ctx, le, b, sfs, so, opTypes, lookupOp, declared, prev, tx)
		if err != nil {
			return nil, err
		}
		if !refused {
			return nil, ErrUnresolvedOp
		}
	}
	return roots, nil
}

// refusedAt reports whether replay refuses tx at the World prev of so, for
// the type of the operation tx runs first: when the type resolves nowhere and
// no plugin prev installs declares it. A nil prev, an unknown World, decides
// nothing. A failure to read prev decides nothing either, as replay would stop
// there rather than refuse. declared holds the declarations read so far, so
// each World and type is read once.
func refusedAt(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	opTypes OpTypes,
	lookupOp world.LookupOp,
	declared map[declaredKey][]string,
	prev *InnerState,
	tx *world_block_tx.Tx,
) (bool, error) {
	// Find the type of the operation the transaction runs first.
	if prev == nil {
		return false, nil
	}
	opTypeID := firstOpType(tx)
	if opTypeID == "" {
		return false, nil
	}
	if op, err := lookupOp(ctx, opTypeID); op != nil || err != nil {
		return false, err
	}

	// Replay refuses every transaction on no World, and an undeclared type
	// on any World.
	if prev.GetHeadRef().GetEmpty() || opTypes.Declared == nil {
		return true, nil
	}
	key := declaredKey{root: prev.GetHeadRef().GetRootRef().MarshalString(), opTypeID: opTypeID}
	pluginIDs, ok := declared[key]
	if !ok {
		var err error
		pluginIDs, err = declaredAt(ctx, le, b, sfs, so, opTypes, lookupOp, prev, opTypeID)
		if err != nil {
			if ctx.Err() != nil {
				return false, context.Cause(ctx)
			}
			return false, nil
		}
		declared[key] = pluginIDs
	}
	return len(pluginIDs) == 0, nil
}

// declaredKey names the declarations of one operation type at the World with
// one root. It holds the root ref alone: the World's head ref also carries its
// transform config, which holds the Space's block key.
type declaredKey struct {
	root, opTypeID string
}

// declaredAt returns the plugins the World state of so, read from its block
// store, declares handle opTypeID.
func declaredAt(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	so sobject.SharedObject,
	opTypes OpTypes,
	lookupOp world.LookupOp,
	state *InnerState,
	opTypeID string,
) ([]string, error) {
	// Open a cursor at the World in the block store of so.
	store := so.GetBlockStore()
	cursor, err := newWorldCursor(ctx, le, b, sfs, store, store.GetID(), store.GetDecodedBlockCache(), state.GetHeadRef())
	if err != nil {
		return nil, err
	}
	defer cursor.Release()

	// Read it through a read-only engine.
	engine, err := world_block.NewEngine(ctx, le, cursor, lookupOp, nil, false)
	if err != nil {
		return nil, err
	}
	defer engine.Close()
	ws, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer ws.Discard()

	// Read its declarations.
	return opTypes.Declared(ctx, ws, opTypeID)
}

// firstOpType returns the type of the applied operation tx runs first, or ""
// when it runs something else first.
func firstOpType(tx *world_block_tx.Tx) string {
	for tx.GetTxType() == world_block_tx.TxType_TxType_BATCH {
		txs := tx.GetTxBatch().GetTxs()
		if len(txs) == 0 {
			return ""
		}
		tx = txs[0]
	}
	switch tx.GetTxType() {
	case world_block_tx.TxType_TxType_APPLY_WORLD_OP:
		return tx.GetTxApplyWorldOp().GetOperationTypeId()
	case world_block_tx.TxType_TxType_APPLY_OBJECT_OP:
		return tx.GetTxApplyObjectOp().GetOperationTypeId()
	}
	return ""
}

// checkpointWorld returns the World the checkpoint of snap carries, and its
// root when it has one.
func checkpointWorld(ctx context.Context, snap sobject.SharedObjectStateSnapshot) (*InnerState, []*block.BlockRef, error) {
	// Decode the checkpoint's World.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, nil, err
	}
	state := &InnerState{}
	if err := state.UnmarshalVT(checkpoint.GetStateData()); err != nil {
		return nil, nil, errors.Wrap(err, "checkpoint World state")
	}

	// Name its root when it has one.
	var roots []*block.BlockRef
	if worldRoot := state.GetHeadRef().GetRootRef(); !worldRoot.GetEmpty() {
		roots = append(roots, worldRoot)
	}
	return state, roots, nil
}

// PublishedGraph returns one root reaching every block a replay of snap reads:
// the World its checkpoint carries and the payloads of the operations above
// it, resolving operation types with lookupOp. With payloads, the root is a new
// HeldRoots block, returned as entry for the caller to store; otherwise it is
// the World root and entry is nil. An operation this participant cannot decode
// or its config does not authorize applies nothing and has no payload.
func PublishedGraph(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (*block.PutBatchEntry, *block.BlockRef, error) {
	// Name the roots, then return the only one or a block naming them all.
	roots, _, err := snapRoots(ctx, snap, lookupOp)
	if err != nil {
		return nil, nil, err
	}
	switch len(roots) {
	case 0:
		return nil, nil, nil
	case 1:
		return nil, roots[0], nil
	}
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return nil, nil, err
	}
	return entry, entry.Ref, nil
}

// snapRoots returns the World of the checkpoint of snap, if any, then the
// payload roots of the operations above it. unresolved reports an operation
// whose type lookupOp did not resolve, whose payloads roots holds only in part.
func snapRoots(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (roots []*block.BlockRef, unresolved bool, err error) {
	// Decode the checkpoint's World.
	_, roots, err = checkpointWorld(ctx, snap)
	if err != nil {
		return nil, false, err
	}

	// Collect the payload roots of the operations above it.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, h := range set.Order() {
		refs, known, err := opPayloads(ctx, snap, set, h, lookupOp)
		if err != nil {
			return nil, false, err
		}
		roots = append(roots, refs...)
		unresolved = unresolved || !known
	}
	return roots, unresolved, nil
}

// opPayloads returns the payload roots of operation h of set, read from snap,
// resolving operation types with lookupOp. An acknowledgment, an equivocated
// operation, one its config does not authorize, and one that does not decode
// apply nothing and have none. known is false when only some payloads could be
// read yet: lookupOp failed for the operation's type, which a plugin may
// register later, and refs holds the roots read without it, the object roots
// the operation set. err is a failure to read snap.
func opPayloads(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet, h []byte, lookupOp world.LookupOp) (refs []*block.BlockRef, known bool, err error) {
	// Skip the operations that apply nothing.
	tx, _, err := replayedTx(ctx, snap, set, h)
	if err != nil || tx == nil {
		return nil, err == nil, err
	}

	// Read the roots the transaction names.
	refs, err = tx.PayloadRefs(ctx, lookupOp)
	return refs, err == nil, nil
}

// replayedTx returns the World transaction operation h of set, read from
// snap, runs, or nil for one that runs none. leaves reports one that leaves
// the World: an acknowledgment, an equivocated operation, one its config does
// not authorize, one that does not decode, a fence and a root set. err is a
// failure to read snap.
func replayedTx(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet, h []byte) (tx *world_block_tx.Tx, leaves bool, err error) {
	// Skip what replay skips whole.
	inner := set.Get(h)
	if inner.IsAcknowledgment() || set.Equivocated(h) {
		return nil, true, nil
	}
	_, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
	if err != nil {
		return nil, false, err
	}

	// Return the transaction of a World operation that runs one.
	op := &WorldOp{}
	if reason != "" || op.UnmarshalVT(opData) != nil {
		return nil, true, nil
	}
	switch op.GetBody().(type) {
	case *WorldOp_ApplyTxOp:
		return op.GetApplyTxOp().GetTx(), false, nil
	case *WorldOp_Fence, *WorldOp_RootSet:
		return nil, true, nil
	}
	return nil, false, nil
}

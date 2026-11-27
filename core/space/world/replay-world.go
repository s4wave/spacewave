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
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// ReplayWorld replays the operation set of snap as the Space World engine
// engineID, and returns the World after it. The pages it computes land in a
// stage on so's block store, which holds them until release, so the caller
// copies the World before releasing the stage.
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

// replayOnce replays snap on a transient adapter whose pages stay in memory up
// to maxPending bytes and go to its replay stage on so's block store past that
// bound. The stage holds them until release. replayOnce waits for the trims the
// adapter's installs start.
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
	// Replay through a transient adapter, which holds what it writes under
	// its replay stage until release.
	a, err := NewAdapter(le, so, AdapterOptions{
		Bus:             b,
		StepFactorySet:  sfs,
		EngineID:        engineID,
		OpTypes:         opTypes,
		MaxPendingBytes: maxPending,
		transient:       true,
	})
	if err != nil {
		return nil, nil, err
	}
	release := a.replay.stage.end
	defer a.replay.close()
	err = sobject.NewBodyDriver(le, so, a, nil, nil).Step(ctx, snap)
	a.waitBackground()
	if err != nil {
		release()
		return nil, nil, err
	}
	return a, release, nil
}

// ErrUnresolvedOp is returned by ImportRoots when the type of an operation
// above the checkpoint does not resolve, such as a plugin's before the plugin
// registers, so its payloads cannot all be identified.
var ErrUnresolvedOp = errors.New("an operation's type does not resolve")

// ImportRoots returns the roots a replay of snap, the state of so, reads, so a
// copy of them lets another device replay the state without the source
// device: the World its checkpoint carries, if any, then the payloads of the
// operations above it.
// It resolves operation types as the Adapter of so for the Space World engine
// engineID on b does, with opTypes, looking each type up on b once.
//
// An operation whose type resolves nowhere identifies its payloads only in
// part, so ImportRoots returns that part where the source's replay refuses it
// and fails with ErrUnresolvedOp otherwise. Replay refuses it when the
// Adapter's saved replay cursor records it as refused at the same position
// above the same checkpoint, and decides one the cursor has not reached at the
// World before it: the World after the cursor's last matching outcome, or the
// checkpoint's World without a matching outcome, kept across the operations so
// decided, which leave the World unchanged. There replay refuses a transaction
// only on a World that is not initialized, when the type of the operation it
// runs first resolves nowhere: a missing handler is a fact about a device.
// Replay waits for the handler instead of refusing the transaction. A failed
// lookup of that type fails ImportRoots with its error, on a World that is not
// initialized and on one that is. An operation whose body this build cannot
// read, and that replay does not refuse before its body, fails ImportRoots with
// ErrUnreadableOp, whatever the cursor saved for it. A refused operation still
// identifies every payload it can, at the cost of bytes the target may not
// read. Invariant: an operation gets no payloads only when every replay of this
// state would refuse it whatever its handlers, or its payloads cannot be
// identified.
func ImportRoots(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
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

	// Identify each operation's payloads, following the saved outcomes
	// while they record the same operations. prev is the World before the
	// operation while it is known.
	prev := base
	for i, h := range set.Order() {
		// Take a saved outcome. A refusal may only mean the source
		// lacked a handler another device has, so a refused operation
		// still identifies its payloads where it can.
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

		// Identify the payloads of an operation replay has not reached.
		// One that runs no transaction has none. The World after an
		// operation that changes the World another way is not known
		// here.
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

		// Look up the type of the operation it runs first at a known
		// World, failing with the lookup's own error. Replay refuses
		// the transaction only on a World that is not initialized, when
		// that type resolves nowhere. On any other World replay waits
		// for a handler.
		if prev == nil {
			return nil, ErrUnresolvedOp
		}
		opTypeID := firstOpType(tx)
		if opTypeID == "" {
			return nil, ErrUnresolvedOp
		}
		op, err := lookupOp(ctx, opTypeID)
		if err != nil {
			return nil, err
		}
		if op != nil || !prev.GetHeadRef().GetEmpty() {
			return nil, ErrUnresolvedOp
		}
	}
	return roots, nil
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

	// Collect the checkpoint World's root when the World has one.
	var roots []*block.BlockRef
	if worldRoot := state.GetHeadRef().GetRootRef(); !worldRoot.GetEmpty() {
		roots = append(roots, worldRoot)
	}
	return state, roots, nil
}

// PublishedGraph returns one root reaching every block a replay of snap reads:
// the World its checkpoint carries and the payloads of the operations above it,
// resolving operation types with lookupOp. With payloads, the root is a new
// HeldRoots block, returned as entry for the caller to store. Otherwise the
// root is the World root and entry is nil. An operation this participant cannot
// decode or its config does not authorize applies nothing and has no payload.
func PublishedGraph(ctx context.Context, snap sobject.SharedObjectStateSnapshot, lookupOp world.LookupOp) (*block.PutBatchEntry, *block.BlockRef, error) {
	// Collect the roots, then return the only one or a block listing them
	// all.
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

// PublishedRoots returns the roots data lists when it is exactly the block
// PublishedGraph builds for a graph of several roots: a HeldRoots of two or
// more roots and nothing else, in the encoding refsEntry writes. It reports
// false for any other bytes.
//
// A reader that holds only a published graph's root tells with it whether the
// root is that block or the one World or payload root the graph has. Those are
// encoded with the Space's transform, and read as this block only if their
// bytes are exactly two or more block refs.
func PublishedRoots(data []byte) ([]*block.BlockRef, bool) {
	// Decode the block and take two or more valid roots.
	held := &HeldRoots{}
	if err := held.UnmarshalVT(data); err != nil || len(held.GetRoots()) < 2 {
		return nil, false
	}
	roots := make([]*block.BlockRef, len(held.GetRoots()))
	for i, root := range held.GetRoots() {
		if root.Validate(false) != nil {
			return nil, false
		}
		h := root.GetHash()
		roots[i] = &block.BlockRef{Hash: &hash.Hash{HashType: h.GetHashType(), Hash: h.GetHash()}}
	}

	// Require data to be the encoding of those roots alone, which rules out
	// an earlier block and any field this type does not define.
	exact, err := (&HeldRoots{Roots: roots}).MarshalVT()
	if err != nil || !bytes.Equal(exact, data) {
		return nil, false
	}
	return roots, true
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
		if errors.Is(err, ErrUnreadableOp) {
			unresolved = true
			continue
		}
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
// the operation set. err is a failure to read snap, or ErrUnreadableOp at a
// body this build cannot read.
func opPayloads(
	ctx context.Context,
	snap sobject.SharedObjectStateSnapshot,
	set *sobject.SOOperationSet,
	h []byte,
	lookupOp world.LookupOp,
) (refs []*block.BlockRef, known bool, err error) {
	// Skip the operations that apply nothing.
	tx, _, err := replayedTx(ctx, snap, set, h)
	if err != nil || tx == nil {
		return nil, err == nil, err
	}

	// Read the roots the transaction references.
	refs, err = tx.PayloadRefs(ctx, lookupOp)
	return refs, err == nil, nil
}

// replayedTx returns the World transaction operation h of set, read from snap,
// runs, or nil for one that runs none. leaves reports one that leaves the
// World: an acknowledgment, one replay refuses before its body, such as an
// equivocated or skipped one, one its config does not authorize, one that does
// not decode, a removed body, a fence and a root set. err is a failure to read
// snap, or ErrUnreadableOp at a body this build cannot read, which may
// reference payloads and change the World.
func replayedTx(ctx context.Context, snap sobject.SharedObjectStateSnapshot, set *sobject.SOOperationSet, h []byte) (tx *world_block_tx.Tx, leaves bool, err error) {
	// Skip what replay skips whole, under the config replay reads.
	inner := set.Get(h)
	if inner.IsAcknowledgment() {
		return nil, true, nil
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, false, err
	}
	if sobject.ReplayRefusal(set, cfg, h, inner) != "" {
		return nil, true, nil
	}
	_, opData, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
	if err != nil {
		return nil, false, err
	}

	// Decide the operation by the fields this build does not declare, then
	// by its body.
	op := &WorldOp{}
	if reason != "" || op.UnmarshalVT(opData) != nil {
		return nil, true, nil
	}
	if reason, err := readUnknownFields(op); err != nil || reason != "" {
		return nil, reason != "", err
	}
	switch op.GetBody().(type) {
	case *WorldOp_ApplyTxOp:
		return op.GetApplyTxOp().GetTx(), false, nil
	case *WorldOp_Fence, *WorldOp_RootSet:
		return nil, true, nil
	}
	return nil, false, nil
}

package sobject_world_engine

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// replayOutcome is the deterministic outcome of one replayed operation.
type replayOutcome struct {
	// hash identifies the operation.
	hash []byte
	// reason is empty when the operation applied. Otherwise it says in plain
	// words why the operation was not applied.
	reason string
}

// replayPosition is one replayed operation and the World after it.
type replayPosition struct {
	// outcome is the outcome of the operation.
	outcome replayOutcome
	// state is the World after the operation.
	state *InnerState
}

// replayer replays an operation set to a World in the set's deterministic
// order. The outcome of every operation depends only on the operation set,
// never on the replaying device or the order operations arrived in. It keeps
// the World after every replayed position, so a later replay resumes after the
// longest prefix its order shares with the previous replay.
type replayer struct {
	// c processes World operations.
	c *Controller
	// so holds the World blocks.
	so sobject.SharedObject
	// base is the World before the first operation.
	base *InnerState
	// decode decodes operation data written with the Space's block transform.
	decode func([]byte) ([]byte, error)
	// config returns the config with the given config chain hash.
	config func(ctx context.Context, hash []byte) (*sobject.SharedObjectConfig, error)
	// positions are the replayed operations in order.
	positions []replayPosition
}

// replay replays set and returns the World after the last operation it can
// place, with the outcome of every placed operation in order.
func (r *replayer) replay(ctx context.Context, set *sobject.SOOperationSet) (*InnerState, []replayOutcome, error) {
	// Keep the positions of the prefix the new order shares with the last one.
	order := set.Order()
	n := 0
	for n < len(order) && n < len(r.positions) && bytes.Equal(order[n], r.positions[n].outcome.hash) {
		n++
	}
	r.positions = r.positions[:n]

	// Replay the rest of the order from the World after the shared prefix.
	state := r.base
	if n != 0 {
		state = r.positions[n-1].state
	}
	equivocated := make(map[string]bool)
	for _, ev := range set.Equivocations() {
		for _, h := range ev.Hashes {
			equivocated[string(h)] = true
		}
	}
	for i, h := range order[n:] {
		var reason string
		if equivocated[string(h)] {
			reason = "its author signed another operation at the same sequence"
		} else {
			next, why, err := r.replayOp(ctx, set.Get(h), n+i, state)
			if err != nil {
				return nil, nil, err
			}
			if next != nil {
				state = next
			}
			reason = why
		}
		r.positions = append(r.positions, replayPosition{
			outcome: replayOutcome{hash: h, reason: reason},
			state:   state,
		})
	}

	// Report every outcome in order.
	outcomes := make([]replayOutcome, len(r.positions))
	for i, pos := range r.positions {
		outcomes[i] = pos.outcome
	}
	return state, outcomes, nil
}

// replayOp applies one operation to state as its author, under the config the
// operation names. It returns the next World, or nil and the reason the
// operation was not applied.
func (r *replayer) replayOp(ctx context.Context, inner *sobject.SOOperationInner, idx int, state *InnerState) (*InnerState, string, error) {
	// Authorize the author under the config the operation was written under.
	author, err := inner.ParsePeerID()
	if err != nil {
		return nil, "", err
	}
	cfg, err := r.config(ctx, inner.GetConfigHash())
	if err != nil {
		return nil, "", err
	}
	var writer *sobject.SOParticipantConfig
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() == inner.GetPeerId() && sobject.CanWriteOps(p.GetRole()) {
			writer = p
			break
		}
	}
	if writer == nil {
		return nil, "its author could not write to the Space", nil
	}

	// Decode the World operation.
	opData, err := r.decode(inner.GetOpData())
	if err != nil {
		return nil, "its data could not be decoded", nil
	}

	// Apply it as the author's person.
	person := writer.GetEntityId()
	if person == "" {
		person = author.String()
	}
	next, res, err := r.c.processOp(
		world.WithOperationPerson(ctx, person),
		r.c.le,
		r.so,
		opData,
		inner.GetLocalId(),
		author,
		inner.GetNonce(),
		idx,
		state,
	)
	if err != nil {
		return nil, "", err
	}
	if !res.GetSuccess() {
		return nil, res.GetErrorDetails().GetErrorMsg(), nil
	}
	return next, "", nil
}

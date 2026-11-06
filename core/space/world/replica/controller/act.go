package space_world_replica_controller

import (
	"context"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
	"github.com/s4wave/spacewave/net/crypto"
)

// act takes step. It keeps this node's invite only while it invites and its
// proof only while it serves one, so the other node never acts on a stale one.
func (r *reconciler) act(ctx context.Context, step space_world_replica.ReplicaStep, obs *space_world_replica.Observation) error {
	// Drop what the step no longer serves.
	state := r.c.getState()
	proof, invite := state.GetProof(), state.GetInvite()
	if step != space_world_replica.ReplicaStep_REPLICA_STEP_SERVE_PROOF {
		proof = nil
	}
	if step != space_world_replica.ReplicaStep_REPLICA_STEP_INVITE {
		invite = nil
	}
	r.c.setState(proof, invite)

	// Follow the head while proving the copy, since the owner accepts only
	// a proof of its head.
	if r.mount != nil {
		follow := step == space_world_replica.ReplicaStep_REPLICA_STEP_BACKFILL ||
			step == space_world_replica.ReplicaStep_REPLICA_STEP_SERVE_PROOF
		if err := r.mount.accessBackfill(ctx, func(be space_world.BackfillEngine) error {
			be.SetBackfillFollowsHead(follow)
			return nil
		}); err != nil {
			return err
		}
	}

	// Take the step. The others wait, or are taken by the provider: the
	// successor's hosting commits the departure, and the departing side
	// follows and serves blocks on its own.
	switch step {
	case space_world_replica.ReplicaStep_REPLICA_STEP_JOIN:
		return r.join(ctx)
	case space_world_replica.ReplicaStep_REPLICA_STEP_BACKFILL:
		return r.backfill(ctx)
	case space_world_replica.ReplicaStep_REPLICA_STEP_SERVE_PROOF:
		return r.serveProof(obs.Backfill)
	case space_world_replica.ReplicaStep_REPLICA_STEP_INVITE:
		return r.invite(ctx)
	case space_world_replica.ReplicaStep_REPLICA_STEP_TRANSFER:
		return r.transfer(ctx)
	}
	return nil
}

// join joins the SharedObject with the other node's invite.
func (r *reconciler) join(ctx context.Context) error {
	invite := r.c.getPeerState().GetInvite()
	if invite == nil {
		return nil
	}
	_, err := r.acc.JoinViaInvite(ctx, r.sessionKey(), invite, "")
	return errors.Wrap(err, "join with the owner's invite")
}

// backfill turns on the copy and asks once for a recheck that trusts no
// earlier proof. A recheck that found holes is asked for again.
func (r *reconciler) backfill(ctx context.Context) error {
	return r.mount.accessBackfill(ctx, func(be space_world.BackfillEngine) error {
		// Turn the copy on.
		if !be.GetBackfill().GetValue() {
			if err := be.SetBackfill(ctx, true); err != nil {
				return err
			}
		}

		// Ask again after an outcome with holes.
		current := be.GetBackfillState().GetValue()
		if fresh := r.freshBackfill(current); fresh != nil && !fresh.IsComplete() {
			r.rechecked = false
		}
		if r.rechecked {
			return nil
		}
		if err := be.RecheckBackfill(ctx); err != nil {
			return err
		}
		r.rechecked, r.recheckBase = true, current
		return nil
	})
}

// serveProof signs a proof of the complete copy bf and serves it, unless the
// served proof already covers bf.
func (r *reconciler) serveProof(bf *space_world.BackfillState) error {
	// Keep a served proof of the same World.
	state := r.c.getState()
	if served := state.GetProof(); served != nil {
		prev := &space_world_replica.SOReplicaProof{}
		if err := prev.UnmarshalVT(served.GetProof()); err == nil && prev.GetSeqno() == bf.GetSeqno() && prev.GetRoot().EqualVT(bf.GetRoot()) {
			return nil
		}
	}

	// Sign and serve a proof of bf.
	signed, err := space_world_replica.SignReplicaProof(r.sessionKey(), &space_world_replica.SOReplicaProof{
		SharedObjectId: r.c.conf.GetSpec().GetSharedObjectId(),
		PeerId:         r.c.local.String(),
		Seqno:          bf.GetSeqno(),
		Root:           bf.GetRoot(),
		CheckedAt:      timestamppb.New(time.Now()),
	})
	if err != nil {
		return err
	}
	r.c.setState(signed, nil)
	return nil
}

// invite prepares an invite targeted to the desired owner and serves it,
// unless one is already served.
func (r *reconciler) invite(ctx context.Context) error {
	// Keep a served invite.
	if r.c.getState().GetInvite() != nil {
		return nil
	}

	// Create the invite and serve it on the Session transport.
	so := r.mount.so
	invite, err := so.CreateSOInviteOp(ctx, so.GetPrivKey(), r.c.conf.GetProviderId(), &sobject.SOInvite{
		Role:         sobject.SOParticipantRole_SOParticipantRole_WRITER,
		TargetPeerId: r.c.conf.GetSpec().GetDesiredOwnerPeerId(),
	})
	if err != nil {
		return err
	}
	if err := r.acc.PrepareDirectInvite(ctx, r.sessionKey(), so.GetPrivKey(), invite); err != nil {
		return err
	}
	r.c.setState(nil, invite)
	return nil
}

// transfer leaves the SharedObject with the desired owner as successor.
func (r *reconciler) transfer(ctx context.Context) error {
	id := r.c.conf.GetSpec().GetSharedObjectId()
	err := r.acc.LeaveSharedObject(ctx, r.sessionKey(), id, r.c.conf.GetSpec().GetDesiredOwnerPeerId())
	return errors.Wrap(err, "transfer to the desired owner")
}

// sessionKey returns the key of the Session this Device runs as.
func (r *reconciler) sessionKey() crypto.PrivKey {
	return r.key
}

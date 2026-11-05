package space_world_replica

// NextStep returns the one step that closes the gap between the spec and the
// observation. It reads only obs, so a pass after a restart picks the same step
// for the same observation.
func NextStep(obs *Observation) ReplicaStep {
	// Act only while both nodes want the same owner of the same object.
	if obs.PeerSpec == nil {
		return ReplicaStep_REPLICA_STEP_WAIT_PEER
	}
	if !obs.Spec.EqualVT(obs.PeerSpec) {
		return ReplicaStep_REPLICA_STEP_CONFLICT
	}

	// The desired owner and every other Device take different steps.
	if obs.LocalPeerID == obs.Spec.GetDesiredOwnerPeerId() {
		return nextDesiredOwnerStep(obs)
	}
	return nextOtherStep(obs)
}

// nextDesiredOwnerStep returns the step of the Device that should own: join,
// copy and prove the copy, then complete the transfer.
func nextDesiredOwnerStep(obs *Observation) ReplicaStep {
	// Without a copy, join with the owner's invite. A departed owner keeps
	// its copy listed but is no longer a participant, so it joins again.
	own := obs.Ownership
	if own == nil || obs.Departing() {
		if obs.InviteReceived {
			return ReplicaStep_REPLICA_STEP_JOIN
		}
		return ReplicaStep_REPLICA_STEP_WAIT_INVITE
	}

	// An owner commits a pending departure, and is otherwise done.
	if obs.Owns() {
		if !own.Settled() {
			return ReplicaStep_REPLICA_STEP_COMPLETE_TRANSFER
		}
		return ReplicaStep_REPLICA_STEP_NONE
	}

	// A member proves its copy complete before the owner transfers.
	if obs.Backfill.IsComplete() {
		return ReplicaStep_REPLICA_STEP_SERVE_PROOF
	}
	return ReplicaStep_REPLICA_STEP_BACKFILL
}

// nextOtherStep returns the step of a Device that should not own: invite the
// desired owner, transfer once it proves its copy, then follow.
func nextOtherStep(obs *Observation) ReplicaStep {
	// A Device with no copy, one that does not own, or one beside an owning
	// desired owner has nothing to do; a departing owner follows.
	own := obs.Ownership
	if own == nil {
		return ReplicaStep_REPLICA_STEP_NONE
	}
	if obs.Departing() {
		return ReplicaStep_REPLICA_STEP_FOLLOW
	}
	desired := obs.Spec.GetDesiredOwnerPeerId()
	if !obs.Owns() || !own.Settled() || own.Owns(desired) {
		return ReplicaStep_REPLICA_STEP_NONE
	}

	// The owner invites the desired owner, then transfers on its proof.
	switch {
	case !obs.DesiredOwnerParticipates:
		return ReplicaStep_REPLICA_STEP_INVITE
	case obs.ProofAccepted:
		return ReplicaStep_REPLICA_STEP_TRANSFER
	default:
		return ReplicaStep_REPLICA_STEP_WAIT_PROOF
	}
}

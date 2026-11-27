// Package forge_execution defines the Execution, one replica of a Pass running
// on one assigned peer.
//
// An Execution records its peer, its Placement, the Target reference, the
// inputs it received and the outputs it writes, its Result and appended log
// entries. Its states are PENDING until the assigned peer acknowledges the
// start, RUNNING, CANCELING while the executor drains what it holds, and
// COMPLETE in success or failure.
//
// A Claim fences the controller instance that runs the Execution. It carries an
// opaque claim ID, a monotonic epoch and a lease (DefaultClaimLease). The
// holder renews the lease. Once it lapses, a peer that observed the expiry may
// reclaim the Execution. Reclaiming raises the epoch and fences the previous
// holder out of every later write. Claimants cancel their Targets
// ClaimClockSkew before the published expiry, so a reclaim does not overlap the
// previous run.
//
// ComputeExecutionOutputs maps an Execution's exec outputs onto the Target's
// declared outputs and requires every replica to have produced equal values.
package forge_execution

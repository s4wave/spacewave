package execution_tx

import (
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	uuid "github.com/satori/go.uuid"
)

// implicitClaim fences legacy transaction callers to the current process. It
// must not become a durable controller ID that survives reconstruction.
var implicitClaim = &forge_execution.Claim{
	ClaimId: uuid.NewV4().String(),
	Epoch:   1,
}

func claimOrImplicit(claims []*forge_execution.Claim) *forge_execution.Claim {
	if len(claims) != 0 && claims[0] != nil {
		return claims[0]
	}
	return implicitClaim
}

func checkClaimEpoch(rootEpoch, claimEpoch uint64) error {
	if claimEpoch != 0 && claimEpoch == rootEpoch {
		return nil
	}
	return &StaleClaimEpochError{
		ClaimEpoch:   claimEpoch,
		CurrentEpoch: rootEpoch,
	}
}

func checkClaim(root *forge_execution.Claim, claimID string, claimEpoch uint64) error {
	if err := checkClaimEpoch(root.GetEpoch(), claimEpoch); err != nil {
		return err
	}
	if claimID == root.GetClaimId() {
		return nil
	}
	return &ClaimHeldError{
		ClaimID: root.GetClaimId(),
		Epoch:   root.GetEpoch(),
	}
}

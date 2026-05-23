package sobject

import (
	"testing"
)

func TestCheckConsensusAcceptance(t *testing.T) {
	// SINGLE_VALIDATOR (default zero value) accepts 1+ signatures.
	if err := CheckConsensusAcceptance(SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR, 1); err != nil {
		t.Fatalf("expected acceptance with 1 sig: %v", err)
	}
	if err := CheckConsensusAcceptance(SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR, 3); err != nil {
		t.Fatalf("expected acceptance with 3 sigs: %v", err)
	}

	// SINGLE_VALIDATOR rejects 0 signatures.
	if err := CheckConsensusAcceptance(SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR, 0); err == nil {
		t.Fatal("expected rejection with 0 sigs")
	}

	// Default zero value == SINGLE_VALIDATOR.
	if err := CheckConsensusAcceptance(0, 1); err != nil {
		t.Fatalf("expected zero value to behave as SINGLE_VALIDATOR: %v", err)
	}

	// Unknown mode returns error.
	if err := CheckConsensusAcceptance(SOConsensusMode(999), 1); err == nil {
		t.Fatal("expected error for unknown consensus mode")
	}
}

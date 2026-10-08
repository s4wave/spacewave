package forge_runtime

import "github.com/pkg/errors"

// CleanupReceipt records the terminal cleanup facts for one reservation generation.
type CleanupReceipt struct {
	// ReservationObjectKey is the released reservation object key.
	ReservationObjectKey string
	// ExecutionObjectKey is the owning Execution object key.
	ExecutionObjectKey string
	// RuntimeIdentity is the stopped runtime identity, empty when no runtime launched.
	RuntimeIdentity string
	// Generation is the fenced generation the receipt applies to.
	Generation uint64
	// RuntimeStopped records that the backend runtime was confirmed stopped,
	// or that no runtime ever launched. It stays false while a stop is pending;
	// the receipt never fabricates this fact.
	RuntimeStopped bool
	// CapacityReleased records that reserved capacity was credited back exactly once.
	CapacityReleased bool
	// Reason records why the reservation released: "stopped" or "expired".
	Reason string
}

// Complete reports whether every cleanup fact is recorded.
func (r *CleanupReceipt) Complete() bool {
	return r != nil && r.RuntimeStopped && r.CapacityReleased
}

// Validate validates the receipt.
func (r *CleanupReceipt) Validate() error {
	if r == nil {
		return errors.New("cleanup receipt cannot be nil")
	}
	switch {
	case r.ReservationObjectKey == "":
		return errors.New("reservation_object_key cannot be empty")
	case r.ExecutionObjectKey == "":
		return errors.New("execution_object_key cannot be empty")
	case r.Generation == 0:
		return errors.New("generation must be set")
	case r.Reason == "":
		return errors.New("reason must be set")
	case r.CapacityReleased != r.RuntimeStopped:
		return errors.New("receipt must be partial (nothing released, stop unknown) or complete (stop confirmed and capacity credited)")
	}
	return nil
}

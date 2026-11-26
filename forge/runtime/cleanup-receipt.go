package forge_runtime

import "github.com/pkg/errors"

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
	case r.Reason == CleanupReason_CLEANUP_REASON_UNKNOWN || !r.Reason.Valid():
		return errors.New("invalid reason")
	case r.CapacityReleased != r.RuntimeStopped:
		return errors.New("receipt must be partial (nothing released, stop unknown) or complete (stop confirmed and capacity credited)")
	}
	return nil
}

// Valid reports whether the reason is a defined value.
func (r CleanupReason) Valid() bool {
	_, ok := CleanupReason_name[int32(r)]
	return ok
}

package viewstream

import "time"

// AdmissionObserver records first-read admission. Calls must be concurrent-safe
// and cannot block on I/O or call back into admission.
type AdmissionObserver interface {
	// Admitted records the priority and queued wait of a granted slot.
	Admitted(priority int, wait time.Duration)
	// Waiting adds delta to the number of queued callers at a priority.
	Waiting(priority, delta int)
	// Expired records a slot returned by its maximum hold deadline.
	Expired()
}

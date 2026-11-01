package block

import "context"

// PublicationReservation owns capacity before a caller takes its writer locks.
// Release returns unconsumed capacity and is safe after admission or repeated
// calls. A reservation belongs to one publisher and can admit one publication.
type PublicationReservation interface {
	// Bytes is the reserved block payload budget.
	Bytes() int64
	// Release returns capacity that no publication consumed.
	Release()
}

// PublicationReserver is an optional bounded publisher capability. ReservePublication
// waits for capacity with cancellation; callers must hold no writer lock while
// calling it. A nil reservation means this publisher has no reservation lane.
type PublicationReserver interface {
	// SupportsPublicationReservations reports whether this lane is enabled.
	SupportsPublicationReservations() bool
	// TryReservePublication claims capacity without waiting, or returns nil if full.
	TryReservePublication(ctx context.Context, bytes int64) (PublicationReservation, error)
	// AcquirePublicationPreparation bounds candidates retained outside writer locks.
	// Acquire before the World writer and release after admission or Discard.
	AcquirePublicationPreparation(ctx context.Context) (func(), error)
	// ReservePublication waits for a nonnegative payload budget without writer locks.
	ReservePublication(ctx context.Context, bytes int64) (PublicationReservation, error)
}

// publicationReservationKey carries a claim to the publisher that owns it.
type publicationReservationKey struct{}

// WithPublicationReservation carries reservation to one SubmitAtomic call.
func WithPublicationReservation(ctx context.Context, reservation PublicationReservation) context.Context {
	return context.WithValue(ctx, publicationReservationKey{}, reservation)
}

// GetPublicationReservation returns the capacity claim carried by ctx.
func GetPublicationReservation(ctx context.Context) PublicationReservation {
	reservation, _ := ctx.Value(publicationReservationKey{}).(PublicationReservation)
	return reservation
}

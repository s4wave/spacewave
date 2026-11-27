package promstorage

import "github.com/pkg/errors"

var (
	// ErrReservedLabel is returned when a sample's labels carry a name that the
	// store's target labels already provide.
	ErrReservedLabel = errors.New("prom: label name is set by the target")
	// ErrCommitInProgress is returned when a store is committed while another
	// commit of the same store has not finished.
	ErrCommitInProgress = errors.New("prom: commit already in progress")
	// ErrStoodDown is returned when another writer took over a store's target.
	ErrStoodDown = errors.New("prom: another Device took over the target")
	// ErrCorruptStore is returned when a target's stored keys do not follow the
	// store layout.
	ErrCorruptStore = errors.New("prom: corrupt store")
	// ErrUnknownEncoding is returned when a stored chunk uses an encoding this
	// version cannot read.
	ErrUnknownEncoding = errors.New("prom: unknown chunk encoding")
)

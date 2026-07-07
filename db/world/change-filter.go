package world

import (
	"context"
	"slices"
)

// ChangeFilter describes World changes that can wake a selective waiter.
type ChangeFilter struct {
	// ObjectKeys matches object changes for exact keys.
	ObjectKeys []string
	// ObjectKeyPrefixes matches object changes under key prefixes.
	ObjectKeyPrefixes []string
	// GraphQuads matches graph changes with non-empty fields as exact filters.
	GraphQuads []GraphQuad
	// AnyObject matches any object change.
	AnyObject bool
	// AnyGraph matches any graph change.
	AnyGraph bool
}

// IsEmpty reports whether the filter accepts any World change.
func (f ChangeFilter) IsEmpty() bool {
	return !f.AnyObject && !f.AnyGraph &&
		len(f.ObjectKeys) == 0 &&
		len(f.ObjectKeyPrefixes) == 0 &&
		len(f.GraphQuads) == 0
}

// Clone returns an independent copy of the filter.
func (f ChangeFilter) Clone() ChangeFilter {
	return ChangeFilter{
		ObjectKeys:        slices.Clone(f.ObjectKeys),
		ObjectKeyPrefixes: slices.Clone(f.ObjectKeyPrefixes),
		GraphQuads:        slices.Clone(f.GraphQuads),
		AnyObject:         f.AnyObject,
		AnyGraph:          f.AnyGraph,
	}
}

// WorldWaitChange allows readers to wait for a matching change after a sequence number.
type WorldWaitChange interface {
	WaitChange(ctx context.Context, afterSeqno uint64, filter ChangeFilter) (uint64, error)
}

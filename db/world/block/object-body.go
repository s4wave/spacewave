package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// objectBodyLookahead is how many body lookups a read-only state runs at once
// for one page, matching the Okra batch walk's loads in flight. A cold page of
// N bodies then waits on about N/16 lookups' dependent reads instead of N.
const objectBodyLookahead = 16

// GetObjectBodiesBatchPage reads one page of object bodies and the seqno of
// this world state, each body through its object. A read-only state overlaps
// the lookups, since its trees serve concurrent reads; a write state reads one
// at a time, as its held writes are not safe to read concurrently.
func (t *WorldState) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	width := objectBodyLookahead
	if t.write {
		width = 1
	}
	return world.LookupObjectBodiesPage(ctx, t, keys, byteBudget, width)
}

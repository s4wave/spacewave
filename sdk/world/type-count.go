package s4wave_world

import "github.com/s4wave/spacewave/db/world"

// TypeCountsFromEntries returns the counts of entries, without their traits.
func TypeCountsFromEntries(entries []*TypeCountEntry) []*world.TypeCount {
	out := make([]*world.TypeCount, len(entries))
	for i, e := range entries {
		out[i] = &world.TypeCount{TypeId: e.GetTypeId(), Count: e.GetCount()}
	}
	return out
}

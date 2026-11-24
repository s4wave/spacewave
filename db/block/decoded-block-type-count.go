package block

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

// DecodedBlockCacheTypeCount is the number and cost of a scope's live entries
// of one block type.
type DecodedBlockCacheTypeCount struct {
	// TypeKey is the block type's DecodedBlockCacheTypeKey.
	TypeKey string
	// Entries is the number of live entries of the type.
	Entries int64
	// Cost is the summed cost of those entries.
	Cost int64
}

// CountByType returns the number and cost of the scope's live entries by
// block type, largest cost first. It walks every entry, so it suits an
// occasional report, not a metric read on each scrape.
func (c *DecodedBlockCache) CountByType() []DecodedBlockCacheTypeCount {
	// Sum the live entries by type.
	byType := make(map[string]*DecodedBlockCacheTypeCount)
	for r := range c.Resident() {
		count := byType[r.TypeKey]
		if count == nil {
			count = &DecodedBlockCacheTypeCount{TypeKey: r.TypeKey}
			byType[r.TypeKey] = count
		}
		count.Entries++
		count.Cost += r.Cost
	}

	// Order the types by cost, then by name.
	counts := make([]DecodedBlockCacheTypeCount, 0, len(byType))
	for count := range maps.Values(byType) {
		counts = append(counts, *count)
	}
	slices.SortFunc(counts, func(a, b DecodedBlockCacheTypeCount) int {
		return cmp.Or(cmp.Compare(b.Cost, a.Cost), strings.Compare(a.TypeKey, b.TypeKey))
	})
	return counts
}

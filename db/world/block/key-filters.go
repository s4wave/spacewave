package world_block

import filters "github.com/s4wave/spacewave/db/block/filters"

// ApplyWorldChangeToKeyFilters applies the world change to the key filter builder.
func ApplyWorldChangeToKeyFilters(b *filters.KeyFiltersBuilder, ch *WorldChange) {
	if k := ch.GetKey(); len(k) != 0 {
		b.ApplyObjectKey(k)
	}
	if gq := ch.GetQuad(); !gq.IsEmpty() {
		b.ApplyQuad(gq)
	}
}

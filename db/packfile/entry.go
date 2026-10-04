package packfile

import "time"

// TrashAge is how long a pack stays trash before a reclaim pass may retire
// it. A writer whose catalog view predates the trash mark has this long to
// commit the blocks it deduplicated against the pack, so the retiring pass
// sees them live and rescues them.
const TrashAge = 24 * time.Hour

// IsSuperseded reports that the pack left the catalog, either replaced by
// another pack or retired from the trash.
func (e *PackfileEntry) IsSuperseded() bool {
	return e.GetSupersededBy() != "" || e.GetSupersededAt() != nil
}

// IsTrash reports that a reclaim pass marked the pack as trash.
func (e *PackfileEntry) IsTrash() bool {
	return e.GetTrashedAt() != nil
}

package git_world

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for HeadRefStore.
func (h *HeadRefStore) BlockAliasIdentity() *block.AliasIdentityToken {
	return &h.unknownFields
}

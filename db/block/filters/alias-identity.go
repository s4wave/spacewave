package filters

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for KeyFilters.
func (w *KeyFilters) BlockAliasIdentity() *block.AliasIdentityToken {
	return &w.unknownFields
}

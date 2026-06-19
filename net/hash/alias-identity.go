package hash

import "github.com/s4wave/spacewave/db/block/aliasidentity"

// BlockAliasIdentity returns the in-memory alias token for Hash.
func (h *Hash) BlockAliasIdentity() *aliasidentity.Token {
	return &h.unknownFields
}

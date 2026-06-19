package world_block

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for ChangeLogLL.
func (w *ChangeLogLL) BlockAliasIdentity() *block.AliasIdentityToken {
	return &w.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for WorldChangeLL.
func (w *WorldChangeLL) BlockAliasIdentity() *block.AliasIdentityToken {
	return &w.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for WorldChange.
func (w *WorldChange) BlockAliasIdentity() *block.AliasIdentityToken {
	return &w.unknownFields
}

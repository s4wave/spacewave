package block_mock

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for Root.
func (r *Root) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for SubBlock.
func (r *SubBlock) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

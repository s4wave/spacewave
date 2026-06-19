package bucket

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for ObjectRef.
func (o *ObjectRef) BlockAliasIdentity() *block.AliasIdentityToken {
	return &o.unknownFields
}

package forge_value

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for Value.
func (v *Value) BlockAliasIdentity() *block.AliasIdentityToken {
	return &v.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Result.
func (r *Result) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for WorldObjectSnapshot.
func (s *WorldObjectSnapshot) BlockAliasIdentity() *block.AliasIdentityToken {
	return &s.unknownFields
}

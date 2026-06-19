package forge_pass

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for ExecState.
func (s *ExecState) BlockAliasIdentity() *block.AliasIdentityToken {
	return &s.unknownFields
}

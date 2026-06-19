package msgpack

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for MsgpackBlob.
func (m *MsgpackBlob) BlockAliasIdentity() *block.AliasIdentityToken {
	return &m.unknownFields
}

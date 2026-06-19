package bldr_manifest

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for ManifestBundle.
func (m *ManifestBundle) BlockAliasIdentity() *block.AliasIdentityToken {
	return &m.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ManifestRef.
func (m *ManifestRef) BlockAliasIdentity() *block.AliasIdentityToken {
	return &m.unknownFields
}

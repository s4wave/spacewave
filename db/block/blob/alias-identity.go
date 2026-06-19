package blob

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for Blob.
func (b *Blob) BlockAliasIdentity() *block.AliasIdentityToken {
	return &b.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ChunkIndex.
func (r *ChunkIndex) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Chunk.
func (r *Chunk) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

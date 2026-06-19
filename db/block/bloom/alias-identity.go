package bloom

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for BloomFilter.
func (b *BloomFilter) BlockAliasIdentity() *block.AliasIdentityToken {
	return &b.unknownFields
}

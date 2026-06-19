package kvtx_block

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for KeyValueStore.
func (k *KeyValueStore) BlockAliasIdentity() *block.AliasIdentityToken {
	return &k.unknownFields
}

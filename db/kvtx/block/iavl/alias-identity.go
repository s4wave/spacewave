package kvtx_block_iavl

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for Node.
func (n *Node) BlockAliasIdentity() *block.AliasIdentityToken {
	return &n.unknownFields
}

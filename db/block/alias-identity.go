package block

import "github.com/s4wave/spacewave/net/hash"

// AliasIdentityToken is an opaque per-instance token used to detect in-memory
// block aliases while marshaling a transaction.
type AliasIdentityToken = hash.AliasIdentityToken

// BlockWithAliasIdentity exposes stable in-memory identity for blocks that can
// appear both as handles and inline sub-blocks.
type BlockWithAliasIdentity interface {
	BlockAliasIdentity() *AliasIdentityToken
}

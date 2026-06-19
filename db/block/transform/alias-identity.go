package block_transform

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for StepConfig.
func (c *StepConfig) BlockAliasIdentity() *block.AliasIdentityToken {
	return &c.unknownFields
}

package world_block_tx

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for Tx.
func (t *Tx) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxBatch.
func (t *TxBatch) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxApplyObjectOp.
func (t *TxApplyObjectOp) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxApplyWorldOp.
func (t *TxApplyWorldOp) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxCreateObject.
func (t *TxCreateObject) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxDeleteGraphQuad.
func (t *TxDeleteGraphQuad) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxDeleteObject.
func (t *TxDeleteObject) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxGCSweep.
func (t *TxGCSweep) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxObjectIncRev.
func (t *TxObjectIncRev) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxObjectSet.
func (t *TxObjectSet) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxRenameObject.
func (t *TxRenameObject) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TxSetGraphQuad.
func (t *TxSetGraphQuad) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

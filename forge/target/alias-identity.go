package forge_target

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for ValueSet.
func (v *ValueSet) BlockAliasIdentity() *block.AliasIdentityToken {
	return &v.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Exec.
func (e *Exec) BlockAliasIdentity() *block.AliasIdentityToken {
	return &e.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Input.
func (i *Input) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Output.
func (o *Output) BlockAliasIdentity() *block.AliasIdentityToken {
	return &o.unknownFields
}

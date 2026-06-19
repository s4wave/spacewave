package file

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for File.
func (f *File) BlockAliasIdentity() *block.AliasIdentityToken {
	return &f.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Range.
func (r *Range) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

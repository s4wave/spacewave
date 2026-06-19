package git_block

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for EncodedObjectStore.
func (r *EncodedObjectStore) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for EndOfIndexEntry.
func (r *EndOfIndexEntry) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Index.
func (i *Index) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for IndexEntry.
func (i *IndexEntry) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ModuleReferencesStore.
func (r *ModuleReferencesStore) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ReferencesStore.
func (r *ReferencesStore) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Reference.
func (r *Reference) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ResolveUndo.
func (i *ResolveUndo) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for ResolveUndoEntry.
func (e *ResolveUndoEntry) BlockAliasIdentity() *block.AliasIdentityToken {
	return &e.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Submodule.
func (r *Submodule) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Tree.
func (i *Tree) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TreeEntry.
func (i *TreeEntry) BlockAliasIdentity() *block.AliasIdentityToken {
	return &i.unknownFields
}

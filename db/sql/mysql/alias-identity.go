package mysql

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for DatabaseRoot.
func (r *DatabaseRoot) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for DatabaseRootTable.
func (r *DatabaseRootTable) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for RootDb.
func (r *RootDb) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TableColumn.
func (t *TableColumn) BlockAliasIdentity() *block.AliasIdentityToken {
	return &t.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for TablePartitionRoot.
func (r *TablePartitionRoot) BlockAliasIdentity() *block.AliasIdentityToken {
	return &r.unknownFields
}

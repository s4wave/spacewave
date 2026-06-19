package unixfs_block

import "github.com/s4wave/spacewave/db/block"

// BlockAliasIdentity returns the in-memory alias token for FSNode.
func (n *FSNode) BlockAliasIdentity() *block.AliasIdentityToken {
	return &n.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for Dirent.
func (d *Dirent) BlockAliasIdentity() *block.AliasIdentityToken {
	return &d.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for FSChange.
func (c *FSChange) BlockAliasIdentity() *block.AliasIdentityToken {
	return &c.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for FSPath.
func (p *FSPath) BlockAliasIdentity() *block.AliasIdentityToken {
	return &p.unknownFields
}

// BlockAliasIdentity returns the in-memory alias token for FSSymlink.
func (s *FSSymlink) BlockAliasIdentity() *block.AliasIdentityToken {
	return &s.unknownFields
}

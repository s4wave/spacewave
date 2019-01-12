package btree

import (
	"github.com/aperturerobotics/pbobject"
)

// GetObjectTypeID returns the object type string, used to identify types.
func (g *Node) GetObjectTypeID() *pbobject.ObjectTypeID {
	return pbobject.NewObjectTypeID("/objstore/btree/node/0.0.1")
}

// GetObjectTypeID returns the object type string, used to identify types.
func (r *Root) GetObjectTypeID() *pbobject.ObjectTypeID {
	return pbobject.NewObjectTypeID("/objstore/btree/root/0.0.1")
}

package mysql

import (
	"strings"

	"github.com/aperturerobotics/hydra/block"
	namedsbset "github.com/aperturerobotics/hydra/block/sbset"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewRootBlock constructs a new root block.
func NewRootBlock() block.Block {
	return &Root{}
}

// LoadRoot follows the root cursor.
// may return nil
func LoadRoot(cursor *block.Cursor) (*Root, error) {
	ni, err := cursor.Unmarshal(NewRootBlock)
	if err != nil {
		return nil, err
	}
	niv, ok := ni.(*Root)
	if !ok || niv == nil {
		return nil, nil
	}
	if err := niv.Validate(); err != nil {
		return nil, err
	}
	return niv, nil
}

// Validate validates the root block.
func (r *Root) Validate() error {
	var prevName string
	for i, ent := range r.GetDatabases() {
		if err := ent.Validate(); err != nil {
			return errors.Wrapf(err, "root databases[%s]", ent.GetName())
		}
		// enforce i[n] < i[n+1]
		if i > 0 {
			if dbname := ent.GetName(); strings.Compare(prevName, dbname) >= 0 {
				return errors.Errorf(
					"databases[%d] %s cannot be after or equal to databases[%d] %s",
					i, dbname,
					i-1, prevName,
				)
			}
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Root) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Root) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (n *Root) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 1:
		sb, ok := next.(*namedsbset.NamedSubBlockSet)
		if !ok {
			return errors.New("unexpected type for root db set field")
		}
		// sb is already configured to reference root.
		_ = sb
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (n *Root) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{
		1: newRootDbsSetContainer(n, nil),
	}
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (n *Root) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return newRootDbsSetContainer(n, nil)
		}
	}
	return nil
}

// GetRootDbSet returns the root db set sub-block.
//
// bcs is optional
func (n *Root) GetRootDbSet(bcs *block.Cursor) *namedsbset.NamedSubBlockSet {
	if bcs != nil {
		bcs = bcs.FollowSubBlock(1)
		b, _ := bcs.GetBlock()
		nrs, ok := b.(*namedsbset.NamedSubBlockSet)
		if !ok || nrs.GetCursor() == nil {
			nrs = newRootDbsSetContainer(n, bcs)
			bcs.SetBlock(nrs)
		}
		return nrs
	}
	return newRootDbsSetContainer(n, nil)
}

// InsertDatabase inserts a new database. Caller should check if it doesn't exist before calling.
//
// Returns new cursor located at *RootDb, added.
// bcs can be nil, or should be located at root of db.
func (n *Root) InsertDatabase(name string, ref *cid.BlockRef, bcs *block.Cursor) (*RootDb, *block.Cursor) {
	set := n.GetRootDbSet(bcs)
	rd := &RootDb{Name: name, Ref: ref}
	n.Databases = append(n.Databases, rd)
	var ebcs *block.Cursor
	if bcs != nil {
		ebcs = set.GetCursor().FollowSubBlock(uint32(len(n.Databases) - 1))
	}
	set.SortNamedRefs()
	return rd, ebcs
}

// _ is a type assertion
var (
	_ block.Block              = ((*Root)(nil))
	_ block.BlockWithSubBlocks = ((*Root)(nil))
)

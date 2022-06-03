package mysql

import (
	"strings"

	"github.com/aperturerobotics/hydra/block"
	namedsbset "github.com/aperturerobotics/hydra/block/sbset"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// NewDatabaseRootBlock constructs a new db root block.
func NewDatabaseRootBlock() block.Block {
	return &DatabaseRoot{}
}

// LoadDatabaseRoot follows the database root cursor.
// may return nil
func LoadDatabaseRoot(cursor *block.Cursor) (*DatabaseRoot, error) {
	ni, err := cursor.Unmarshal(NewDatabaseRootBlock)
	if err != nil {
		return nil, err
	}
	niv, ok := ni.(*DatabaseRoot)
	if !ok || niv == nil {
		return nil, nil
	}
	if err := niv.Validate(); err != nil {
		return nil, err
	}
	return niv, nil
}

// Validate validates the database root block.
func (r *DatabaseRoot) Validate() error {
	var prevName string
	for i, table := range r.GetTables() {
		if err := table.Validate(); err != nil {
			return errors.Wrapf(err, "tables[%s]", table.GetName())
		}
		// enforce i[n] < i[n+1]
		if i > 0 {
			if dbname := table.GetName(); strings.Compare(prevName, dbname) >= 0 {
				return errors.Errorf(
					"tables[%d] %s cannot be after or equal to tables[%d] %s",
					i, dbname,
					i-1, prevName,
				)
			}
		}
	}
	return nil
}

// Validate performs cursory validation.
func (t *DatabaseRootTable) Validate() error {
	if len(t.GetName()) == 0 {
		return ErrEmptyTableName
	}
	if err := t.GetRef().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *DatabaseRoot) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *DatabaseRoot) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplySubBlock applies a sub-block change with a field id.
func (r *DatabaseRoot) ApplySubBlock(id uint32, next block.SubBlock) error {
	// noop
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (r *DatabaseRoot) GetSubBlocks() map[uint32]block.SubBlock {
	return map[uint32]block.SubBlock{
		1: newDbRootTableSetContainer(r, nil),
	}
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (r *DatabaseRoot) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return newDbRootTableSetContainer(r, nil)
		}
	}
	return nil
}

// GetRootTableSet returns the root table set sub-block.
//
// bcs is optional
func (r *DatabaseRoot) GetRootTableSet(bcs *block.Cursor) *namedsbset.NamedSubBlockSet {
	if bcs != nil {
		bcs = bcs.FollowSubBlock(1)
		b, _ := bcs.GetBlock()
		nrs, ok := b.(*namedsbset.NamedSubBlockSet)
		if !ok || nrs.GetCursor() == nil {
			nrs = newDbRootTableSetContainer(r, bcs)
			bcs.SetBlock(nrs, true)
		}
		return nrs
	}
	return newDbRootTableSetContainer(r, nil)
}

// InsertTable inserts a new table. Caller should check if it doesn't exist before calling.
//
// Returns new cursor, added.
// bcs can be nil, or should be located at root.
func (r *DatabaseRoot) InsertTable(name string, ref *block.BlockRef, bcs *block.Cursor) (*block.Cursor, bool) {
	set := r.GetRootTableSet(bcs)
	r.Tables = append(r.Tables, &DatabaseRootTable{Name: name, Ref: ref})
	var ebcs *block.Cursor
	if bcs != nil {
		ebcs = set.GetCursor().FollowSubBlock(uint32(len(r.Tables) - 1))
	}
	set.SortNamedRefs()
	return ebcs, true
}

// _ is a type assertion
var (
	_ block.Block              = ((*DatabaseRoot)(nil))
	_ block.BlockWithSubBlocks = ((*DatabaseRoot)(nil))
)

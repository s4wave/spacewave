package world

import "github.com/s4wave/spacewave/db/block"

// TitledBlock is an object body that names its object for listings. A type
// whose body implements it shows that title in object browsers; a type
// without it shows the object key.
type TitledBlock interface {
	block.Block
	// WorldTitle returns the object's title, or empty to show the key.
	WorldTitle() string
}

// InternalBlock marks a type whose objects are bookkeeping a person did not
// make. Object browsers hide internal types by default.
type InternalBlock interface {
	block.Block
	// WorldInternal marks the type internal.
	WorldInternal()
}

// BlockTitle returns the title blk declares, or empty when its type declares
// none.
func BlockTitle(blk block.Block) string {
	if t, ok := blk.(TitledBlock); ok {
		return t.WorldTitle()
	}
	return ""
}

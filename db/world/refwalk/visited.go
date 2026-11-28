package refwalk

import (
	"context"
	"reflect"

	"github.com/s4wave/spacewave/db/block"
)

// VisitKey identifies a block as a walk read it: its ref and the type it
// decoded the block as.
type VisitKey struct {
	// Ref addresses the block.
	Ref *block.BlockRef
	// Type names the type the walk read the block as, as TypeName does, or is
	// empty for a block read by scanning.
	Type string
}

// Visited records the blocks walks finished, so a Walker reads each block
// once per type across walks. A walk finishes a block once it handed the
// block's read to its visitor and finished, or found finished, every block it
// queued below it. So a finished key stands for its whole subtree, and a set
// that outlives the Walker, such as one stored beside the blocks, lets a later
// walk resume: it skips the finished subtrees and reads again only the blocks
// the stopped walk had not finished.
//
// A Walker calls a Visited from one goroutine.
type Visited interface {
	// Finished reports, for each key, whether a walk finished it.
	Finished(ctx context.Context, keys []VisitKey) ([]bool, error)
	// Finish records that a walk finished key.
	Finish(ctx context.Context, key VisitKey) error
	// Reached reports whether a walk finished ref under a type, and whether
	// one finished it by scanning.
	Reached(ctx context.Context, ref *block.BlockRef) (typed, scanned bool, err error)
}

// TypeName names t for a VisitKey by its package path and name, so two types
// of one name in different packages differ.
func TypeName(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return "*" + TypeName(t.Elem())
	}
	if t.PkgPath() == "" {
		return t.String()
	}
	return t.PkgPath() + "." + t.Name()
}

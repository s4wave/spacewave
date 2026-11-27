package world

import (
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
)

// TitledBlock is an object body that provides its object's title for listings.
// A type whose body implements it shows that title in object browsers. A type
// without it shows the object key.
type TitledBlock interface {
	block.Block
	// WorldTitle returns the object's title, or empty to show the key.
	WorldTitle() string
}

// StatefulBlock is an object body that declares its object's state for
// listings, such as running, passed or failed. Object browsers show it beside
// the title. A type without it shows no state.
type StatefulBlock interface {
	block.Block
	// WorldState returns the object's state in lower case, or empty for none.
	WorldState() string
}

// DatedBlock is an object body that carries the time its object was made.
// Object browsers show the object's age from it.
type DatedBlock interface {
	block.Block
	// WorldTime returns when the object was made, or nil when unknown.
	WorldTime() *timestamppb.Timestamp
}

// InternalBlock marks a type whose objects are bookkeeping a person did not
// make. Object browsers hide internal types by default.
type InternalBlock interface {
	block.Block
	// WorldInternal marks the type internal.
	WorldInternal()
}

// Measure is one summable quantity of an object, such as a receipt's cost.
type Measure struct {
	// Name identifies the quantity across objects, such as "cost" or "tokens".
	Name string
	// Unit is how the sum reads, such as "usd". Empty counts the named
	// things.
	Unit string
	// Value is the object's amount.
	Value float64
}

// MeasuredBlock is an object body that carries summable quantities. Object
// browsers sum them by name over a family of objects, such as the cost and
// tokens of a session's inference receipts.
type MeasuredBlock interface {
	block.Block
	// WorldMeasures returns the object's quantities.
	WorldMeasures() []Measure
}

// BlockTitle returns the title blk declares, or empty when its type declares
// none.
func BlockTitle(blk block.Block) string {
	if t, ok := blk.(TitledBlock); ok {
		return t.WorldTitle()
	}
	return ""
}

// BlockState returns the state blk declares, or empty when its type declares
// none.
func BlockState(blk block.Block) string {
	if s, ok := blk.(StatefulBlock); ok {
		return s.WorldState()
	}
	return ""
}

// BlockTime returns the time blk declares, or nil when its type declares none.
func BlockTime(blk block.Block) *timestamppb.Timestamp {
	if d, ok := blk.(DatedBlock); ok {
		return d.WorldTime()
	}
	return nil
}

// BlockMeasures returns the quantities blk carries, or nil when its type
// carries none.
func BlockMeasures(blk block.Block) []Measure {
	if m, ok := blk.(MeasuredBlock); ok {
		return m.WorldMeasures()
	}
	return nil
}

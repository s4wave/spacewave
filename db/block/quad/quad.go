package quad

import (
	"github.com/s4wave/spacewave/db/block"
)

// NewQuadBlock constructs a new object block.
func NewQuadBlock() block.Block {
	return &Quad{}
}

// IsEmpty checks if the graph quad is empty.
// Considered empty if subject, predicate, or object fields are empty.
func (o *Quad) IsEmpty() bool {
	return o.GetSubject() == "" ||
		o.GetPredicate() == "" ||
		o.GetObj() == ""
}

// Clone clones the graph quad.
func (o *Quad) Clone() *Quad {
	if o == nil {
		return nil
	}
	return &Quad{
		Subject:   o.Subject,
		Predicate: o.Predicate,
		Obj:       o.Obj,
		Label:     o.Label,
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *Quad) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *Quad) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*Quad)(nil))

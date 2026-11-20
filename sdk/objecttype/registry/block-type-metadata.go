package s4wave_objecttype_registry

import (
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/world"
)

// BlockTypeMetadata returns the metadata a root block type declares through
// the World trait interfaces: its traits, and internal visibility for a
// world.InternalBlock. A plugin registers it with the type, so the host knows
// which bodies to ask DeclareBodies about without decoding any.
func BlockTypeMetadata(bt blocktype.BlockType) *ObjectTypeMetadata {
	// Read the traits off a zero body.
	blk := bt.Constructor()
	_, titled := blk.(world.TitledBlock)
	_, stateful := blk.(world.StatefulBlock)
	_, dated := blk.(world.DatedBlock)
	_, measured := blk.(world.MeasuredBlock)
	md := &ObjectTypeMetadata{Traits: &ObjectTypeTraits{
		Titled:   titled,
		Stateful: stateful,
		Dated:    dated,
		Measured: measured,
	}}

	// Hide bookkeeping types from routine browsing.
	if _, internal := blk.(world.InternalBlock); internal {
		md.Visibility = ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_INTERNAL
	}
	return md
}

// Declares reports traits that promise anything a listing shows.
func (t *ObjectTypeTraits) Declares() bool {
	return t.GetTitled() || t.GetStateful() || t.GetDated() || t.GetMeasured()
}

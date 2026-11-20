package s4wave_world

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/world"
)

// DeclareBlock returns what blk declares about its object through the World
// trait interfaces, such as world.TitledBlock and world.MeasuredBlock.
func DeclareBlock(blk block.Block) *ObjectDeclaration {
	d := &ObjectDeclaration{
		Title: world.BlockTitle(blk),
		State: world.BlockState(blk),
		Time:  world.BlockTime(blk),
	}
	for _, m := range world.BlockMeasures(blk) {
		d.Measures = append(d.Measures, &ObjectMeasure{Name: m.Name, Unit: m.Unit, Value: m.Value})
	}
	return d
}

// DeclareBodies decodes each root body with bt and returns what it declares,
// in body order. An empty or undecodable body yields an empty declaration.
func DeclareBodies(bt blocktype.BlockType, bodies [][]byte) []*ObjectDeclaration {
	out := make([]*ObjectDeclaration, len(bodies))
	for i, body := range bodies {
		blk := bt.Constructor()
		if len(body) == 0 || blk.UnmarshalVT(body) != nil {
			out[i] = &ObjectDeclaration{}
			continue
		}
		out[i] = DeclareBlock(blk)
	}
	return out
}

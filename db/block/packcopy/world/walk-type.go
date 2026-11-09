package packcopy_world

import (
	"reflect"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// walkCtors are the block types a saved walk position names by Go type: the
// types a World's refs name. A block of another type is saved untyped, and a
// resume types it from its parent's decode.
var walkCtors = []block.Ctor{
	world_block.NewWorldBlock,
	world_block.NewObjectBlock,
	world_block.NewWorldChangeLLBlock,
	world_block.NewChangeLogLLBlock,
	kvtx_block_okra.NewRootBlock,
	kvtx_block_okra.NewPageBlock,
	kvtx_block_iavl.NewNodeBlock,
	blob.NewBlobBlock,
	byteslice.NewByteSliceBlock,
	block_transform.NewTransformConfigBlock,
}

// ctorNames caches the Go type each constructor builds, by its code pointer.
var ctorNames sync.Map

// ctorName returns the Go type ctor builds.
func ctorName(ctor block.Ctor) string {
	// Take the name cached for the constructor's code.
	ptr := reflect.ValueOf(ctor).Pointer()
	if name, ok := ctorNames.Load(ptr); ok {
		return name.(string)
	}

	// Build a block to learn its type, and cache it.
	name := reflect.TypeOf(ctor()).String()
	ctorNames.Store(ptr, name)
	return name
}

// walkRegistry maps the Go types a saved position may name to their
// constructors: walkCtors and the root types of the walk's objects.
func walkRegistry(roots RootTypes) map[string]block.Ctor {
	reg := make(map[string]block.Ctor, len(walkCtors)+len(roots))
	for _, ctor := range walkCtors {
		reg[ctorName(ctor)] = ctor
	}
	for _, ctor := range roots {
		if ctor != nil {
			reg[ctorName(ctor)] = ctor
		}
	}
	return reg
}

// typeSaver saves block types for one state file, collecting the ctors and
// transform configs the header lists.
type typeSaver struct {
	// reg is what a resume can rebuild.
	reg map[string]block.Ctor
	// ctors numbers the ctor names from 1.
	ctors map[string]uint32
	// ctorList lists the ctor names in order.
	ctorList []string
	// xfrms numbers the transform configs from 1.
	xfrms map[*block_transform.Config]uint32
	// xfrmList lists the marshaled configs in order.
	xfrmList [][]byte
}

// newTypeSaver builds a typeSaver over reg.
func newTypeSaver(reg map[string]block.Ctor) *typeSaver {
	return &typeSaver{
		reg:   reg,
		ctors: make(map[string]uint32),
		xfrms: make(map[*block_transform.Config]uint32),
	}
}

// savableType reports whether a resume can rebuild typ from its saved form:
// no type, or one whose ctors reg names.
func savableType(reg map[string]block.Ctor, typ any) bool {
	// No type saves as untyped; a type of another typer does not save.
	if typ == nil {
		return true
	}
	nt, ok := typ.(*nodeType)
	if !ok || nt == nil {
		return false
	}

	// Each constructor must be one reg names.
	if _, ok := reg[ctorName(nt.ctor)]; !ok {
		return false
	}
	if nt.values != nil {
		if _, ok := reg[ctorName(nt.values)]; !ok {
			return false
		}
	}
	return true
}

// save returns typ's saved form, or nil when a resume cannot rebuild it.
func (s *typeSaver) save(typ any) (*WalkType, error) {
	// An untyped block saves as no ctor.
	if !savableType(s.reg, typ) {
		return nil, nil
	}
	if typ == nil {
		return &WalkType{}, nil
	}

	// Number the type's ctors and transform.
	nt := typ.(*nodeType)
	wt := &WalkType{Ctor: s.ctor(nt.ctor), ObjectRoot: nt.objectRoot}
	if nt.values != nil {
		wt.Values = s.ctor(nt.values)
	}
	if nt.xfrm != nil {
		id, ok := s.xfrms[nt.xfrmConf]
		if !ok {
			data, err := nt.xfrmConf.MarshalVT()
			if err != nil {
				return nil, err
			}
			s.xfrmList = append(s.xfrmList, data)
			id = uint32(len(s.xfrmList)) //nolint:gosec // few configs.
			s.xfrms[nt.xfrmConf] = id
		}
		wt.Xfrm = id
	}
	return wt, nil
}

// ctor numbers a ctor's name from 1.
func (s *typeSaver) ctor(ctor block.Ctor) uint32 {
	name := ctorName(ctor)
	id, ok := s.ctors[name]
	if !ok {
		s.ctorList = append(s.ctorList, name)
		id = uint32(len(s.ctorList)) //nolint:gosec // few ctors.
		s.ctors[name] = id
	}
	return id
}

// typeLoader rebuilds the block types a state file saved.
type typeLoader struct {
	// typer builds the transforms.
	typer *Typer
	// reg maps ctor names to constructors.
	reg map[string]block.Ctor
	// ctors are the header's ctor names.
	ctors []string
	// xfrms are the header's transform configs.
	xfrms [][]byte
	// built caches the transforms by number.
	built map[uint32]xfrmPair
}

// xfrmPair is a transform and the config it was built from.
type xfrmPair struct {
	// xfrm is the transform.
	xfrm block.Transformer
	// conf is its config.
	conf *block_transform.Config
}

// load rebuilds a saved type. It reports false for a type this build cannot
// rebuild, which the resume takes from the parent instead.
func (l *typeLoader) load(wt *WalkType) (any, bool) {
	// No saved type is a type to take from the parent; no ctor is an
	// untyped block.
	if wt == nil {
		return nil, false
	}
	if wt.GetCtor() == 0 {
		return nil, true
	}

	// Look up the ctors by name.
	ctor := l.lookup(wt.GetCtor())
	if ctor == nil {
		return nil, false
	}
	nt := &nodeType{ctor: ctor, objectRoot: wt.GetObjectRoot()}
	if wt.GetValues() != 0 {
		if nt.values = l.lookup(wt.GetValues()); nt.values == nil {
			return nil, false
		}
	}

	// Build the transform once per config.
	if id := wt.GetXfrm(); id != 0 {
		pair, ok := l.built[id]
		if !ok {
			if id > uint32(len(l.xfrms)) { //nolint:gosec // few configs.
				return nil, false
			}
			conf := &block_transform.Config{}
			if err := conf.UnmarshalVT(l.xfrms[id-1]); err != nil {
				return nil, false
			}
			xfrm, err := l.typer.transformer(conf)
			if err != nil {
				return nil, false
			}
			pair = xfrmPair{xfrm: xfrm, conf: conf}
			l.built[id] = pair
		}
		nt.xfrm, nt.xfrmConf = pair.xfrm, pair.conf
	}
	return nt, true
}

// lookup returns the constructor numbered id in the header, nil for none.
func (l *typeLoader) lookup(id uint32) block.Ctor {
	if id == 0 || id > uint32(len(l.ctors)) { //nolint:gosec // few ctors.
		return nil
	}
	return l.reg[l.ctors[id-1]]
}

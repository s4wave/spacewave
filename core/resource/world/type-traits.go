package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/world"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// declareFunc reads what root bodies of one type declare, in body order.
type declareFunc func(ctx context.Context, bodies [][]byte) ([]*s4wave_world.ObjectDeclaration, error)

// typeTraits is what a World type's bodies declare about their objects.
type typeTraits struct {
	// traits are the type's declared traits (nil when nobody declares any).
	traits *s4wave_objecttype_registry.ObjectTypeTraits
	// internal reports a type of bookkeeping objects browsers hide.
	internal bool
	// declare reads bodies' declarations (nil when traits declare nothing).
	declare declareFunc
}

// declares reports a type whose bodies declare anything a listing shows.
func (t typeTraits) declares() bool {
	return t.declare != nil && t.traits.Declares()
}

// lookupTypeTraits resolves typeID's traits. A host block type declares them
// through the World trait interfaces and the host decodes its bodies. A type a
// plugin registered declares them in its registration metadata and the plugin
// decodes its bodies. A type nobody knows declares nothing, so its objects
// list by key and it is not internal.
func lookupTypeTraits(ctx context.Context, b bus.Bus, typeID string) (typeTraits, error) {
	// A Resource mounted without a bus knows no types.
	if b == nil {
		return typeTraits{}, nil
	}

	// Decode in the host when its block types include typeID. A block type is
	// a stateless constructor, so it outlives the directive reference.
	btv, _, ref, err := bus.ExecOneOffTyped[blocktype.LookupBlockTypeValue](ctx, b, blocktype.NewLookupBlockType(typeID), bus.ReturnWhenIdle(), nil)
	if err != nil {
		return typeTraits{}, err
	}
	if ref != nil {
		ref.Release()
	}
	if btv != nil {
		bt := btv.GetValue()
		return newTypeTraits(s4wave_objecttype_registry.BlockTypeMetadata(bt), func(_ context.Context, bodies [][]byte) ([]*s4wave_world.ObjectDeclaration, error) {
			return s4wave_world.DeclareBodies(bt, bodies), nil
		}), nil
	}

	// Otherwise read the registering plugin's metadata from the registry.
	md, err := s4wave_objecttype_registry.ExLookupObjectTypeMetadata(ctx, b, typeID)
	if err != nil || md == nil {
		return typeTraits{}, err
	}
	return newTypeTraits(md, func(ctx context.Context, bodies [][]byte) ([]*s4wave_world.ObjectDeclaration, error) {
		return declarePluginBodies(ctx, b, typeID, bodies)
	}), nil
}

// newTypeTraits builds the traits md declares, read through declare.
func newTypeTraits(md *s4wave_objecttype_registry.ObjectTypeMetadata, declare declareFunc) typeTraits {
	return typeTraits{
		traits:   md.GetTraits(),
		internal: md.GetVisibility() == s4wave_objecttype_registry.ObjectTypeVisibility_OBJECT_TYPE_VISIBILITY_INTERNAL,
		declare:  declare,
	}
}

// declarePluginBodies asks the plugin that registered typeID what bodies
// declare. A plugin that is gone or fails declares nothing, so one failing
// plugin leaves only its own objects listed by key.
func declarePluginBodies(ctx context.Context, b bus.Bus, typeID string, bodies [][]byte) ([]*s4wave_world.ObjectDeclaration, error) {
	// Find the registered type. A plugin that is gone is not registered.
	ot, otRef, err := objecttype.ExLookupObjectType(ctx, b, typeID)
	if err != nil {
		return nil, err
	}
	if otRef != nil {
		defer otRef.Release()
	}

	// Ask its owner, treating a failure as nothing declared.
	if d, ok := ot.(objecttype.BodyDeclarer); ok {
		out, err := d.DeclareBodies(ctx, bodies)
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return s4wave_objecttype_registry.EmptyDeclarations(len(bodies)), nil
}

// typeTraitCache resolves each type once for one call or watch.
type typeTraitCache struct {
	b bus.Bus
	m map[string]typeTraits
}

// newTypeTraitCache builds an empty cache over the bus.
func newTypeTraitCache(b bus.Bus) *typeTraitCache {
	return &typeTraitCache{b: b, m: make(map[string]typeTraits)}
}

// get returns the traits of typeID, resolving it on first use.
func (c *typeTraitCache) get(ctx context.Context, typeID string) (typeTraits, error) {
	// Reuse a type resolved earlier in this call or watch.
	if tr, ok := c.m[typeID]; ok {
		return tr, nil
	}

	// Resolve the type once and remember it.
	tr, err := lookupTypeTraits(ctx, c.b, typeID)
	if err != nil {
		return typeTraits{}, err
	}
	c.m[typeID] = tr
	return tr, nil
}

// entries returns counts as SDK entries marked with their types' traits.
func (c *typeTraitCache) entries(ctx context.Context, counts []*world.TypeCount) ([]*s4wave_world.TypeCountEntry, error) {
	out := make([]*s4wave_world.TypeCountEntry, len(counts))
	for i, tc := range counts {
		tr, err := c.get(ctx, tc.GetTypeId())
		if err != nil {
			return nil, err
		}
		out[i] = &s4wave_world.TypeCountEntry{TypeId: tc.GetTypeId(), Count: tc.GetCount(), Internal: tr.internal}
	}
	return out, nil
}

// listTypeCountEntries returns the type counts of ws with their traits.
func listTypeCountEntries(ctx context.Context, b bus.Bus, ws world.WorldState) ([]*s4wave_world.TypeCountEntry, error) {
	counts, err := ws.ListTypeCounts(ctx)
	if err != nil {
		return nil, err
	}
	return newTypeTraitCache(b).entries(ctx, counts)
}

// watchTypeCountEntries sends every type count of ws with its traits, then the
// counts each change moves, as world.WatchTypeCounts steps them.
func watchTypeCountEntries(ctx context.Context, b bus.Bus, ws world.WorldState, send func(*s4wave_world.WatchTypeCountsResponse) error) error {
	traits := newTypeTraitCache(b)
	return world.WatchTypeCounts(ctx, ws, func(seqno uint64, counts []*world.TypeCount) error {
		entries, err := traits.entries(ctx, counts)
		if err != nil {
			return err
		}
		return send(&s4wave_world.WatchTypeCountsResponse{TypeCounts: entries, Seqno: seqno})
	})
}

// objectDeclarations returns what each key's body declares, in key order, when
// the type declares anything, and nil otherwise. It reads each budgeted page
// of bodies once and hands the page to the type's owner in one call. A
// missing or withheld body declares nothing, so the client shows its key.
func objectDeclarations(ctx context.Context, ws world.WorldState, tr typeTraits, keys []string) ([]*s4wave_world.ObjectDeclaration, error) {
	if !tr.declares() || len(keys) == 0 {
		return nil, nil
	}
	out := make([]*s4wave_world.ObjectDeclaration, 0, len(keys))
	for start := 0; start < len(keys); {
		// Read one budgeted page of bodies.
		bodies, consumed, _, err := ws.GetObjectBodiesBatchPage(ctx, keys[start:], objectBodiesBatchBudget)
		if err != nil {
			return nil, err
		}

		// Declare the page's present bodies in one call.
		var raw [][]byte
		var at []int
		for i, body := range bodies {
			if body.Exists && len(body.Body) != 0 {
				raw = append(raw, body.Body)
				at = append(at, i)
			}
		}
		page := s4wave_objecttype_registry.EmptyDeclarations(len(bodies))
		if len(raw) != 0 {
			ds, err := tr.declare(ctx, raw)
			if err != nil {
				return nil, err
			}
			for k, i := range at {
				if k < len(ds) && ds[k] != nil {
					page[i] = ds[k]
				}
			}
		}
		out = append(out, page...)
		if consumed == 0 {
			break
		}
		start += int(consumed)
	}
	return out, nil
}

// declarationTitles returns the titles of ds when the type declares titles.
func declarationTitles(tr typeTraits, ds []*s4wave_world.ObjectDeclaration) []string {
	if !tr.traits.GetTitled() || ds == nil {
		return nil
	}
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.GetTitle()
	}
	return out
}

// declarationStates returns the states of ds when the type declares states.
func declarationStates(tr typeTraits, ds []*s4wave_world.ObjectDeclaration) []string {
	if !tr.traits.GetStateful() || ds == nil {
		return nil
	}
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.GetState()
	}
	return out
}

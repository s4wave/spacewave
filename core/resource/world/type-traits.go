package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// typeTraits is what a World type's body declares about its objects.
type typeTraits struct {
	// bt constructs the type's body; nil when the host does not know the type.
	bt blocktype.BlockType
	// internal reports a type of bookkeeping objects browsers hide.
	internal bool
	// titled reports a body that names its object.
	titled bool
}

// lookupTypeTraits resolves typeID through the host block type lookup. A type
// the host does not know, such as one a plugin owns, declares nothing, so its
// objects list by key and it is not internal.
func lookupTypeTraits(ctx context.Context, b bus.Bus, typeID string) (typeTraits, error) {
	// A Resource mounted without a bus knows no types.
	if b == nil {
		return typeTraits{}, nil
	}

	// Resolve the block type; a block type is a stateless constructor, so it
	// outlives the directive reference.
	btv, _, ref, err := bus.ExecOneOffTyped[blocktype.LookupBlockTypeValue](ctx, b, blocktype.NewLookupBlockType(typeID), bus.ReturnWhenIdle(), nil)
	if err != nil {
		return typeTraits{}, err
	}
	if ref != nil {
		ref.Release()
	}
	if btv == nil {
		return typeTraits{}, nil
	}

	// Read the declarations off a zero body.
	bt := btv.GetValue()
	blk := bt.Constructor()
	_, internal := blk.(world.InternalBlock)
	_, titled := blk.(world.TitledBlock)
	return typeTraits{bt: bt, internal: internal, titled: titled}, nil
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

// objectTitles returns the title of each key, read from its body, when the
// type declares titles, and nil otherwise. A missing, withheld or undecodable
// body has an empty title, so the client shows its key.
func objectTitles(ctx context.Context, ws world.WorldState, tr typeTraits, keys []string) ([]string, error) {
	if !tr.titled || len(keys) == 0 {
		return nil, nil
	}
	titles := make([]string, 0, len(keys))
	for start := 0; start < len(keys); {
		// Read one budgeted page of bodies.
		bodies, consumed, _, err := ws.GetObjectBodiesBatchPage(ctx, keys[start:], objectBodiesBatchBudget)
		if err != nil {
			return nil, err
		}

		// Decode each body and take its title.
		for _, body := range bodies {
			var title string
			if body.Exists && len(body.Body) != 0 {
				blk := tr.bt.Constructor()
				if blk.UnmarshalVT(body.Body) == nil {
					title = world.BlockTitle(blk)
				}
			}
			titles = append(titles, title)
		}
		if consumed == 0 {
			break
		}
		start += int(consumed)
	}
	return titles, nil
}

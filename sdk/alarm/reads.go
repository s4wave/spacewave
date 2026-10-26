package s4wave_alarm

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/world"
)

// ReadsPredicate links an Alarm to each object its watch reads, so a surface
// showing one of those objects can find the Alarms about it.
const ReadsPredicate = "<alarm/reads>"

// SetReads makes the Alarm key's reads edges point at exactly keys. Each key
// must exist; an edge to a deleted object leaves with the object.
func SetReads(ctx context.Context, ws world.WorldState, key string, keys []string) error {
	// Read the current edges.
	found, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, ReadsPredicate, "", ""), 0)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(found))
	for _, q := range found {
		obj, err := world.GraphValueToKey(q.GetObj())
		if err != nil {
			return err
		}
		have[obj] = true
	}

	// Remove the edges to objects no longer read.
	for obj := range have {
		if slices.Contains(keys, obj) {
			continue
		}
		if err := ws.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys(key, ReadsPredicate, obj, "")); err != nil {
			return err
		}
	}

	// Add the missing edges.
	for _, obj := range keys {
		if have[obj] {
			continue
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, ReadsPredicate, obj, "")); err != nil {
			return err
		}
	}
	return nil
}

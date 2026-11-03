package viewcache

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"

	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// typedKeyIndex holds the keys of one type's objects as listed at a revision,
// for the next listing to patch from the type edges changed since.
type typedKeyIndex struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// at is the revision keys were listed at; zero before the first listing.
	at uint64
	// keys holds the sorted keys, read-only once stored.
	keys []string
}

// TypedKeys returns a reader of the keys of every object of typeID. With a
// cache that still holds the changes since its last listing, it checks only
// the objects those changes named, instead of reading every type edge again:
// the full listing reads one log entry per object, which grows with every
// object ever created.
func (c *Cache) TypedKeys(typeID string) func(context.Context, world.WorldState) ([]string, error) {
	return func(ctx context.Context, tx world.WorldState) ([]string, error) {
		// Read directly without a cache or find the revision's type index.
		if c == nil {
			return objectsWithType(ctx, tx, typeID)
		}
		seqno, err := tx.GetSeqno(ctx)
		if err != nil {
			return nil, err
		}
		index := c.typedIndex(typeID)

		// Patch the last listing when the cache covers the changes since.
		index.mtx.Lock()
		at, keys := index.at, index.keys
		index.mtx.Unlock()
		if at != 0 && at <= seqno {
			if changes, ok := c.changesSince(at); ok {
				next, err := patchTypedKeys(ctx, tx, typeID, keys, changes)
				if err != nil {
					return nil, err
				}
				index.store(seqno, next)
				return next, nil
			}
		}

		// Otherwise list every object of the type.
		next, err := objectsWithType(ctx, tx, typeID)
		if err != nil {
			return nil, err
		}
		index.store(seqno, next)
		return next, nil
	}
}

// store keeps keys as the listing at seqno unless a newer one is kept.
func (i *typedKeyIndex) store(seqno uint64, keys []string) {
	i.mtx.Lock()
	if seqno >= i.at {
		i.at, i.keys = seqno, keys
	}
	i.mtx.Unlock()
}

// typedIndex returns the key index of typeID, creating it on first use.
func (c *Cache) typedIndex(typeID string) *typedKeyIndex {
	// Share one synchronized index per object type.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.typed == nil {
		c.typed = make(map[string]*typedKeyIndex)
	}
	index := c.typed[typeID]
	if index == nil {
		index = &typedKeyIndex{}
		c.typed[typeID] = index
	}
	return index
}

// changesSince merges the changes the cache passed after revision at. It
// reports false when the history no longer reaches back to at or the merged
// changes are unknown. The changes may run past a reader's revision; each
// object they name is then checked in the reader's snapshot.
func (c *Cache) changesSince(at uint64) (*world.ChangeSet, bool) {
	// Merge retained changes only while history covers the requested revision.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if at >= c.seqno.Load() {
		return &world.ChangeSet{}, true
	}
	if len(c.history) == 0 || c.history[0].from > at {
		return nil, false
	}
	var changes *world.ChangeSet
	for _, step := range c.history {
		if step.through > at {
			changes = world.MergeChangeSets(changes, step.changes, changeMergeLimit)
		}
	}
	if changes == nil {
		return &world.ChangeSet{}, true
	}
	return changes, !changes.Unknown
}

// patchTypedKeys returns keys with each object the changes named checked
// again in tx: a changed type edge to typeID, or a changed key already listed.
// It records the type's edge pattern in tx's reads, as the full listing does.
func patchTypedKeys(ctx context.Context, tx world.WorldState, typeID string, keys []string, changes *world.ChangeSet) ([]string, error) {
	// Record the pattern, so the value drops when a type edge changes.
	filter := world.NewGraphQuadWithKeys("", world_types.TypePred.String(), world_types.BuildTypeObjectKey(typeID), "")
	if _, err := tx.LookupGraphQuads(ctx, filter, 1); err != nil {
		return nil, err
	}

	// Collect the objects the changes may have typed or untyped.
	check := make(map[string]struct{})
	for _, q := range changes.Quads {
		if q.GetPredicate() != filter.GetPredicate() || q.GetObj() != filter.GetObj() {
			continue
		}
		key, err := iriKey(q.GetSubject())
		if err != nil {
			return nil, err
		}
		check[key] = struct{}{}
	}
	for _, key := range changes.Keys {
		if _, ok := slices.BinarySearch(keys, key); ok {
			check[key] = struct{}{}
		}
	}
	if len(check) == 0 {
		return keys, nil
	}

	// Check each one's type edge in tx and rebuild the sorted keys.
	next := slices.Clone(keys)
	for key := range check {
		edge := world.NewGraphQuadWithKeys(key, filter.GetPredicate(), world_types.BuildTypeObjectKey(typeID), "")
		found, err := tx.LookupGraphQuads(ctx, edge, 1)
		if err != nil {
			return nil, err
		}
		pos, listed := slices.BinarySearch(next, key)
		switch {
		case len(found) != 0 && !listed:
			next = slices.Insert(next, pos, key)
		case len(found) == 0 && listed:
			next = slices.Delete(next, pos, pos+1)
		}
	}
	return next, nil
}

// objectsWithType lists typed object keys through one indexed quad pattern.
func objectsWithType(ctx context.Context, tx world.WorldState, typeID string) ([]string, error) {
	// Follow only the selected type's object-indexed edges.
	filter := world.NewGraphQuadWithKeys("", world_types.TypePred.String(), world_types.BuildTypeObjectKey(typeID), "")
	edges, err := tx.LookupGraphQuads(ctx, filter, 0)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(edges))
	for _, edge := range edges {
		key, err := iriKey(edge.GetSubject())
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, nil
}

// iriKey decodes an object-key relationship endpoint.
func iriKey(v string) (string, error) {
	// Reject a relationship endpoint that does not name an object key.
	iri, ok := quad.StringToValue(v).(quad.IRI)
	if !ok {
		return "", errors.New("relationship endpoint is not an object key")
	}
	return string(iri), nil
}

package viewcache

import (
	"context"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// entryParts collects the cached values one computation reuses.
type entryParts struct {
	// cache is the cache the computation's value is kept in; values of
	// another World's cache are not parts, as its changes do not reach it.
	cache *Cache
	// mtx guards entries, as a computation may read its parts concurrently.
	mtx     sync.Mutex
	entries []*revisionEntry
	// facets are the parts the computation declared it reads by facet.
	facets []facetDeclaration
}

// facetDeclaration is a computation's claim that it reads the cached value
// name only through changed.
type facetDeclaration struct {
	name    string
	compute func(context.Context, world.WorldState) (any, error)
	changed func(previous, next any) bool
}

// partsKey carries the entryParts of the computation running in a context.
type partsKey struct{}

// addViewReads makes the evaluation in ctx depend on entry, a value of c. A
// computation of another value of c records entry as a part, keeping its reads
// by reference: copying them would cost every value built from a large part,
// such as each player's listing built from the route standings, the whole
// part's keys. Any other evaluation adds entry's merged read set. Either way
// the hold of ctx keeps entry's, so the blocks behind a value stay held while
// a value or view built from it lives. A view reading a value that reaches a
// part by facet waits on the value's retirement in place of its reads.
func (c *Cache) addViewReads(ctx context.Context, entry *revisionEntry) {
	block.RefHoldFromContext(ctx).Keep(entry.blocks)
	if parts, ok := ctx.Value(partsKey{}).(*entryParts); ok && parts.cache == c {
		parts.mtx.Lock()
		parts.entries = append(parts.entries, entry)
		parts.mtx.Unlock()
		world.RerunAt(ctx, entry.wake)
		return
	}
	if retired, following, ok := c.retirement(ctx, entry); ok {
		world.RerunOn(ctx, retired)
		world.RerunOn(ctx, following)
		world.RerunAt(ctx, entry.wake)
		return
	}
	world.AddViewReads(ctx, entry.viewReads(), entry.wake)
}

// retirement returns what a view evaluation in ctx waits on in place of
// entry's reads: a context that ends when the cache stops keeping entry, and
// one that ends when the change watch stops feeding the cache. ok is false,
// and the view joins entry's reads, unless entry reaches a part read by
// facet, the cache keeps it finished, the watch feeds the cache (Follow), and
// ctx is not computing a value of any cache.
//
// Such a view needs none of entry's reads. Each catch-up checks entry's own
// reads, its parts and its facets, and drops it when they may have changed
// it, so its retirement wakes the view exactly when a change gets through
// the facets. Joining entry's merged reads instead wakes the view on every
// change to a faceted part, such as every idle player's view on every lobby
// change. Only the watch runs catch-ups for a view that waits without reading,
// so the view also wakes when the watch stops. A computation of another cache
// keeps the reads, since a cache checks reads, not rerun contexts.
func (c *Cache) retirement(ctx context.Context, entry *revisionEntry) (retired, following context.Context, ok bool) {
	// Only a view of a done value reaching a facet goes further, checked
	// without the mtx: every cached read a view makes passes here, and a
	// value's parts and facets are set before done closes.
	if computing(ctx) {
		return nil, nil, false
	}
	select {
	case <-entry.done:
	default:
		return nil, nil, false
	}
	if _, free := entry.checkedSet(); free {
		return nil, nil, false
	}

	// The cache must keep the value while the watch follows.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if !entry.finished || c.entries[entry.name] != entry || c.following == nil {
		return nil, nil, false
	}

	// The first such view creates the context the cache ends on removal.
	if entry.retired == nil {
		entry.retired, entry.retire = context.WithCancel(context.Background())
	}
	return entry.retired, c.following, true
}

// withParts returns a context that collects the cached values computed or
// reused in it, in place of adding their reads to the evaluation. Use it on
// the world.WithViewReads child of the value they are parts of.
func (c *Cache) withParts(ctx context.Context) (context.Context, *entryParts) {
	parts := &entryParts{cache: c}
	return context.WithValue(ctx, partsKey{}, parts), parts
}

// collected returns the parts collected, once the computation ended, with
// the ones it declared a facet of read apart. A part read several times is
// read by facet only when every read declared one, and changes for the
// computation when any declaration reports a change; a read without a
// declaration may need all of it, and so may a name read as two values.
func (p *entryParts) collected() ([]*revisionEntry, []facetRead) {
	// Group repeated part reads and their declared facets.
	p.mtx.Lock()
	defer p.mtx.Unlock()
	reads := make(map[string]int, len(p.entries))
	values := make(map[string]*revisionEntry, len(p.entries))
	mixed := make(map[string]bool)
	for _, entry := range p.entries {
		reads[entry.name]++
		if prev, ok := values[entry.name]; ok && prev != entry {
			mixed[entry.name] = true
		}
		values[entry.name] = entry
	}

	// Group declarations by the same value name.
	declared := make(map[string][]facetDeclaration, len(p.facets))
	for _, decl := range p.facets {
		declared[decl.name] = append(declared[decl.name], decl)
	}

	// Keep a plain dependency unless every read names an identical facet.
	var entries []*revisionEntry
	var facets []facetRead
	seen := make(map[*revisionEntry]bool, len(p.entries))
	for _, entry := range p.entries {
		if seen[entry] {
			continue
		}
		seen[entry] = true
		decls := declared[entry.name]
		if len(decls) == 0 || len(decls) != reads[entry.name] || mixed[entry.name] {
			entries = append(entries, entry)
			continue
		}
		facets = append(facets, facetRead{part: entry, compute: decls[0].compute, changed: anyChanged(decls), reads: entry.viewReads()})
	}
	return entries, facets
}

// anyChanged joins the comparisons of one part's declarations: the part
// changed for the computation when it changed for any of its reads.
func anyChanged(decls []facetDeclaration) func(previous, next any) bool {
	if len(decls) == 1 {
		return decls[0].changed
	}
	return func(previous, next any) bool {
		return slices.ContainsFunc(decls, func(decl facetDeclaration) bool {
			return decl.changed(previous, next)
		})
	}
}

// Facet declares that the computation in ctx reads the cached value name only
// through changed, which compares two values of it for the sub-keys the
// computation read. A catch-up that touches the value then computes it again
// with compute and keeps the computation's value when changed reports no
// difference. Call it after reading name, in a computation of the cache c.
func (c *Cache) Facet(ctx context.Context, name string, compute func(context.Context, world.WorldState) (any, error), changed func(previous, next any) bool) {
	// Attach the facet to the computation currently recording its parts.
	parts, ok := ctx.Value(partsKey{}).(*entryParts)
	if !ok || parts.cache != c {
		return
	}
	parts.mtx.Lock()
	parts.facets = append(parts.facets, facetDeclaration{name: name, compute: compute, changed: changed})
	parts.mtx.Unlock()
}

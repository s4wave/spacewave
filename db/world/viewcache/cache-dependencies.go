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
// a value or view built from it lives.
func (c *Cache) addViewReads(ctx context.Context, entry *revisionEntry) {
	block.RefHoldFromContext(ctx).Keep(entry.blocks)
	if parts, ok := ctx.Value(partsKey{}).(*entryParts); ok && parts.cache == c {
		parts.mtx.Lock()
		parts.entries = append(parts.entries, entry)
		parts.mtx.Unlock()
		world.RerunAt(ctx, entry.wake)
		return
	}
	world.AddViewReads(ctx, entry.viewReads(), entry.wake)
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

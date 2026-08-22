package world

import (
	"context"
	"io"
	"slices"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/graph/iterator"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
)

// QuadEqual checks if two quads are equal.
func QuadEqual(q1, q2 quad.Quad) bool {
	// TODO faster check
	return q1.String() == q2.String()
}

// CheckQuadExists checks if the quad exists on the graph handle.
func CheckQuadExists(ctx context.Context, h CayleyHandle, gq quad.Quad) (bool, error) {
	// Scan matching quads until the requested quad is found.
	var found bool
	err := FilterIterateQuads(ctx, h, gq, func(q quad.Quad) error {
		if q.IsValid() && QuadEqual(q, gq) {
			found = true
			return io.EOF
		}
		return nil
	})

	// Normalize the early-stop sentinel into a successful lookup.
	if err == io.EOF {
		err = nil
	}
	return found, err
}

// FilterIterateQuads iterates over quads matching the input quad.
// Empty fields are ignored.
func FilterIterateQuads(ctx context.Context, h CayleyHandle, gq quad.Quad, cb func(q quad.Quad) error) error {
	return IterateFilteredFullQuads(ctx, h, gq, cb)
}

// IterateFilteredFullQuads iterates over full quads matching a concrete quad filter.
func IterateFilteredFullQuads(ctx context.Context, h CayleyHandle, filter quad.Quad, cb func(q quad.Quad) error) error {
	// Leave empty callbacks and filters to the cheapest valid path.
	if cb == nil {
		return nil
	}
	if !hasQuadFilter(filter) {
		it := h.QuadsAllIterator(ctx).Iterate(ctx)
		defer it.Close()
		return iterateQuadResults(ctx, h, it, cb)
	}

	// Select the smallest indexed direction for the concrete filter.
	dir, ref, ok, err := selectQuadFilterIterator(ctx, h, filter)
	if err != nil || !ok {
		return err
	}

	// Filter the selected iterator before forwarding each matching quad.
	it := h.QuadIterator(ctx, dir, ref).Iterate(ctx)
	defer it.Close()
	return iterateQuadResults(ctx, h, it, func(q quad.Quad) error {
		if !quadMatchesFilter(q, filter) {
			return nil
		}
		return cb(q)
	})
}

// CollectFilteredFullQuadsBatch collects full quads for a batch of concrete quad filters.
func CollectFilteredFullQuadsBatch(ctx context.Context, h CayleyHandle, filters []quad.Quad, limitPerFilter uint32) ([][]quad.Quad, error) {
	// Allocate result slots and group filters by their selected iterator.
	results := make([][]quad.Quad, len(filters))
	groupsByKey := make(map[quadFilterBatchKey]int)
	var groups []quadFilterBatchGroup

	for i, filter := range filters {
		if !hasQuadFilter(filter) {
			quads, err := collectFilteredFullQuads(ctx, h, filter, limitPerFilter)
			if err != nil {
				return nil, err
			}
			results[i] = quads
			continue
		}
		dir, ref, ok, err := selectQuadFilterIterator(ctx, h, filter)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		key := quadFilterBatchKey{dir: dir, ref: graphRefCacheKey(ref)}
		if groupIdx, ok := groupsByKey[key]; ok {
			groups[groupIdx].filterIndexes = append(groups[groupIdx].filterIndexes, i)
			continue
		}
		groupsByKey[key] = len(groups)
		groups = append(groups, quadFilterBatchGroup{
			dir:           dir,
			ref:           ref,
			filterIndexes: []int{i},
		})
	}

	// Scan each shared iterator group and populate its result slots.
	for _, group := range groups {
		if err := collectQuadFilterBatchGroup(ctx, h, filters, results, group, limitPerFilter); err != nil {
			return nil, err
		}
	}
	return results, nil
}

type quadFilterBatchKey struct {
	dir quad.Direction
	ref any
}

type quadFilterBatchGroup struct {
	dir           quad.Direction
	ref           graph.Ref
	filterIndexes []int
}

func collectFilteredFullQuads(ctx context.Context, h CayleyHandle, filter quad.Quad, limit uint32) ([]quad.Quad, error) {
	// Collect matching quads until the requested limit or iterator end.
	var quads []quad.Quad
	err := IterateFilteredFullQuads(ctx, h, filter, func(q quad.Quad) error {
		quads = append(quads, q)

		if limit != 0 && uint32(len(quads)) >= limit { //nolint:gosec
			return io.EOF
		}
		return nil
	})

	// Normalize the early-stop sentinel into a successful collection.
	if err == io.EOF {
		err = nil
	}
	return quads, err
}

func collectQuadFilterBatchGroup(
	ctx context.Context,
	h CayleyHandle,
	filters []quad.Quad,
	results [][]quad.Quad,
	group quadFilterBatchGroup,
	limit uint32,
) error {
	// Open the shared iterator and initialize each filter's limit state.
	it := h.QuadIterator(ctx, group.dir, group.ref).Iterate(ctx)
	defer it.Close()

	var remaining int
	filled := make([]bool, len(filters))
	if limit != 0 {
		remaining = len(group.filterIndexes)
	}

	// Route each scanned quad to every matching filter still below its limit.
	err := iterateQuadResults(ctx, h, it, func(q quad.Quad) error {
		for _, filterIdx := range group.filterIndexes {
			if filled[filterIdx] || !quadMatchesFilter(q, filters[filterIdx]) {
				continue
			}
			results[filterIdx] = append(results[filterIdx], q)

			if limit != 0 && uint32(len(results[filterIdx])) >= limit { //nolint:gosec
				filled[filterIdx] = true
				remaining--
			}
		}
		if limit != 0 && remaining == 0 {
			return io.EOF
		}
		return nil
	})

	// Normalize the early-stop sentinel after all requested filters are filled.
	if err == io.EOF {
		err = nil
	}
	return err
}

// IterateFullQuads iterates over the full quads matched by a shape.
func IterateFullQuads(ctx context.Context, h CayleyHandle, sh shape.Shape, cb func(q quad.Quad) error) error {
	if cb == nil {
		return nil
	}

	// Do not call shape.Optimize here. Optimized shapes may yield node/value refs
	// instead of quad refs, but graph.NewResultReader requires each iterator result
	// to be a quad ref so QuadStore.Quad can recover all four directions.
	it := sh.BuildIterator(ctx, h).Iterate(ctx)
	defer it.Close()
	return iterateQuadResults(ctx, h, it, cb)
}

func iterateQuadResults(ctx context.Context, h CayleyHandle, it iterator.Scanner, cb func(q quad.Quad) error) error {
	// Read quads from the scanner and forward each one to the callback.
	rsc := graph.NewResultReader(ctx, h, it)
	for {
		q, err := rsc.ReadQuad(ctx)
		if err == nil {
			err = cb(q)
		}
		if err != nil {
			// End-of-stream is a successful completion for this iterator.
			if err == io.EOF {
				err = nil
			}
			return err
		}
	}
}

// NewReadOperationCayleyHandle wraps a Cayley handle for a batched read operation.
//
// The returned handle caches ValueOf, NameOf, QuadIteratorSize, and
// QuadDirection results for one read operation. It does not cache resolved
// quad refs; batch reads resolve each quad ref at most once, so caching them
// only costs memory.
func NewReadOperationCayleyHandle(h CayleyHandle) CayleyHandle {
	return &cachedCayleyHandle{
		CayleyHandle:        h,
		valueRefs:           make(map[string]graph.Ref),
		valueRefFound:       make(map[string]bool),
		names:               make(map[any]quad.Value),
		nameFound:           make(map[any]bool),
		quadIteratorSizes:   make(map[quadIteratorSizeKey]refs.Size),
		quadIteratorErrors:  make(map[quadIteratorSizeKey]error),
		quadDirections:      make(map[quadDirectionKey]graph.Ref),
		quadDirectionErrors: make(map[quadDirectionKey]error),
	}
}

type cachedCayleyHandle struct {
	CayleyHandle

	valueRefs           map[string]graph.Ref
	valueRefFound       map[string]bool
	names               map[any]quad.Value
	nameFound           map[any]bool
	quadIteratorSizes   map[quadIteratorSizeKey]refs.Size
	quadIteratorErrors  map[quadIteratorSizeKey]error
	quadDirections      map[quadDirectionKey]graph.Ref
	quadDirectionErrors map[quadDirectionKey]error
}

type quadIteratorSizeKey struct {
	dir quad.Direction
	ref any
}

type quadDirectionKey struct {
	ref any
	dir quad.Direction
}

func (h *cachedCayleyHandle) ValueOf(ctx context.Context, val quad.Value) (graph.Ref, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := graphValueCacheKey(val)
	if h.valueRefFound[key] {
		return h.valueRefs[key], nil
	}
	ref, err := h.CayleyHandle.ValueOf(ctx, val)
	if err != nil {
		return nil, err
	}
	h.valueRefFound[key] = true
	h.valueRefs[key] = ref
	return ref, nil
}

func (h *cachedCayleyHandle) NameOf(ctx context.Context, ref graph.Ref) (quad.Value, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := graphRefCacheKey(ref)
	if h.nameFound[key] {
		return h.names[key], nil
	}
	val, err := h.CayleyHandle.NameOf(ctx, ref)
	if err != nil {
		return nil, err
	}
	h.nameFound[key] = true
	h.names[key] = val
	return val, nil
}

func (h *cachedCayleyHandle) QuadIteratorSize(ctx context.Context, dir quad.Direction, ref graph.Ref) (refs.Size, error) {
	if err := ctx.Err(); err != nil {
		return refs.Size{}, err
	}
	key := quadIteratorSizeKey{dir: dir, ref: graphRefCacheKey(ref)}
	if size, ok := h.quadIteratorSizes[key]; ok {
		return size, h.quadIteratorErrors[key]
	}
	size, err := h.CayleyHandle.QuadIteratorSize(ctx, dir, ref)
	h.quadIteratorSizes[key] = size
	h.quadIteratorErrors[key] = err
	return size, err
}

func (h *cachedCayleyHandle) Quad(ctx context.Context, ref graph.Ref) (quad.Quad, error) {
	if err := ctx.Err(); err != nil {
		return quad.Quad{}, err
	}
	q, ok, err := h.quadFromDirections(ctx, ref)
	if err == nil && !ok {
		q, err = h.CayleyHandle.Quad(ctx, ref)
	}
	return q, err
}

// quadFromDirections resolves a quad ref through its four directions.
// Direction refs are resolved on every Quad call; NameOf remains cached
// within the read operation.
func (h *cachedCayleyHandle) quadFromDirections(ctx context.Context, ref graph.Ref) (quad.Quad, bool, error) {
	var q quad.Quad
	for _, dir := range quad.Directions {
		dirRef, err := h.CayleyHandle.QuadDirection(ctx, ref, dir)
		if err != nil {
			return q, false, err
		}
		if dirRef == nil {
			if dir == quad.Label {
				continue
			}
			return q, false, nil
		}
		val, err := h.NameOf(ctx, dirRef)
		if err != nil {
			return q, false, err
		}
		q.Set(dir, val)
	}
	return q, true, nil
}

func (h *cachedCayleyHandle) QuadDirection(ctx context.Context, ref graph.Ref, dir quad.Direction) (graph.Ref, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := quadDirectionKey{ref: graphRefCacheKey(ref), dir: dir}
	if out, ok := h.quadDirections[key]; ok {
		return out, h.quadDirectionErrors[key]
	}
	out, err := h.CayleyHandle.QuadDirection(ctx, ref, dir)
	h.quadDirections[key] = out
	h.quadDirectionErrors[key] = err
	return out, err
}

func graphValueCacheKey(val quad.Value) string {
	if val == nil {
		return ""
	}
	return val.String()
}

func graphRefCacheKey(ref graph.Ref) any {
	if ref == nil {
		return nil
	}
	return ref.Key()
}

func selectQuadFilterIterator(ctx context.Context, h CayleyHandle, filter quad.Quad) (quad.Direction, graph.Ref, bool, error) {
	var bestDir quad.Direction
	var bestRef graph.Ref
	var bestSize refs.Size
	var found bool
	for _, dir := range quadFilterIteratorDirections(filter) {
		val := filter.Get(dir)
		if val == nil {
			continue
		}
		ref, err := h.ValueOf(ctx, val)
		if err != nil || ref == nil {
			return 0, nil, false, err
		}
		size, err := h.QuadIteratorSize(ctx, dir, ref)
		if err != nil {
			return 0, nil, false, err
		}
		if !found ||
			size.Value < bestSize.Value ||
			(size.Value == bestSize.Value && quadFilterDirectionPriority(dir) < quadFilterDirectionPriority(bestDir)) {
			bestDir = dir
			bestRef = ref
			bestSize = size
			found = true
		}
	}
	return bestDir, bestRef, found, nil
}

func quadFilterIteratorDirections(filter quad.Quad) []quad.Direction {
	// Cayley kv has Subject and Object prefix indexes, while Predicate-only
	// filters fall back to a full quad scan. Keep endpoint directions ahead of
	// Predicate when estimated iterator sizes tie.
	preferred := []quad.Direction{quad.Subject, quad.Object, quad.Predicate, quad.Label}
	out := make([]quad.Direction, 0, len(preferred))
	for _, dir := range preferred {
		if filter.Get(dir) != nil {
			out = append(out, dir)
		}
	}
	return out
}

func quadFilterDirectionPriority(dir quad.Direction) int {
	switch dir {
	case quad.Subject:
		return 0
	case quad.Object:
		return 1
	case quad.Predicate:
		return 2
	case quad.Label:
		return 3
	default:
		return 4
	}
}

func hasQuadFilter(q quad.Quad) bool {
	for _, dir := range quad.Directions {
		if q.Get(dir) != nil {
			return true
		}
	}
	return false
}

func quadMatchesFilter(q, filter quad.Quad) bool {
	for _, dir := range quad.Directions {
		val := filter.Get(dir)
		if val == nil {
			continue
		}
		if !quadValuesEqual(q.Get(dir), val) {
			return false
		}
	}
	return true
}

func quadValuesEqual(v1, v2 quad.Value) bool {
	if v1 == nil || v2 == nil {
		return v1 == v2
	}
	return v1.String() == v2.String()
}

// IteratePathWithKeys starts & iterates a path from the given object keys.
func IteratePathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
	valueCb func(objKey string) (ctnu bool, err error),
) error {
	if valueCb == nil || len(entityKeys) == 0 {
		return nil
	}

	gv := make([]quad.Value, len(entityKeys))
	for i, ek := range entityKeys {
		gv[i] = KeyToGraphValue(ek)
	}

	return ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h CayleyHandle) error {
		p := cayley.StartPath(h, gv...)
		if pathCb != nil {
			var err error
			p, err = pathCb(p)
			if err != nil || p == nil {
				return err
			}
		}

		it := p.BuildIterator(ctx).Iterate(ctx)
		defer it.Close()
		for it.Next(ctx) {
			res, err := it.Result(ctx)
			if err != nil {
				return err
			}
			qv, err := h.NameOf(ctx, res)
			if err != nil {
				return err
			}
			key, err := QuadValueToKey(qv)
			if err != nil {
				return err
			}
			ctnu, err := valueCb(key)
			if err != nil || !ctnu {
				return err
			}
		}
		return it.Err()
	})
}

// CollectPathWithKeys collects the object keys for a given path starting at entityKeys.
//
// If the entityKeys list is empty, returns nil, nil.
func CollectPathWithKeys(
	ctx context.Context,
	ws WorldStateGraph,
	entityKeys []string,
	pathCb func(p *cayley.Path) (*cayley.Path, error),
) ([]string, error) {
	if len(entityKeys) == 0 {
		return nil, nil
	}

	var output []string
	seen := make(map[string]struct{})
	err := IteratePathWithKeys(
		ctx,
		ws,
		entityKeys,
		pathCb,
		func(objKey string) (ctnu bool, err error) {
			if _, ok := seen[objKey]; !ok {
				seen[objKey] = struct{}{}
				output = append(output, objKey)
			}
			return true, nil
		},
	)
	slices.Sort(output)
	return output, err
}

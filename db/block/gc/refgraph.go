package block_gc

import (
	"context"
	"io"
	"maps"
	"sync"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/graph/refs"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// RefGraph is a unified reference graph for garbage collection backed by Cayley.
type RefGraph struct {
	handle *cayley.Handle

	writeMu    sync.Mutex
	mu         sync.Mutex
	iriRefKeys map[string]any
}

const (
	refGraphApplyBatchLimit = 512
	refGraphApplySliceLimit = 4096
)

// refBatchError reports a failed ownership transition together with the
// uncommitted suffix that the caller must retain for a later attempt.
// The slices are always expressed in the original add-before-remove order.
type refBatchError struct {
	err     error
	adds    []RefEdge
	removes []RefEdge
}

func (e *refBatchError) Error() string {
	return e.err.Error()
}

func (e *refBatchError) Unwrap() error {
	return e.err
}

// NewRefGraph constructs a RefGraph backed by the given kvtx store.
// prefix is prepended to all keys (e.g., "gc/" for space context).
func NewRefGraph(ctx context.Context, store kvtx.Store, prefix []byte) (*RefGraph, error) {
	prefixed := kvtx_prefixer.NewPrefixer(store, prefix)
	opts := graph.Options{
		"ignore_duplicate": true,
		"ignore_missing":   true,
		// RefGraph always uses Cayley's default index set; skip reading the
		// index metadata on every world-state rebuild.
		cayley_kv.OptAssumeDefaultIdx: true,
	}
	h, err := kvtx_cayley.NewGraph(ctx, prefixed, opts)
	if err != nil {
		return nil, errors.Wrap(err, "new ref graph")
	}
	return &RefGraph{handle: h}, nil
}

// RegisterEntityChain registers a chain of gc/ref edges between nodes.
// Each adjacent pair gets an AddRef call: nodes[0]->nodes[1],
// nodes[1]->nodes[2], etc. At least 2 nodes required. Idempotent
// (Cayley ignore_duplicate).
func RegisterEntityChain(ctx context.Context, rg RefGraphOps, nodes ...string) error {
	if len(nodes) < 2 {
		return errors.New("RegisterEntityChain requires at least 2 nodes")
	}
	for i := 0; i < len(nodes)-1; i++ {
		if err := rg.AddRef(ctx, nodes[i], nodes[i+1]); err != nil {
			return err
		}
	}
	return nil
}

// AddRef adds a gc/ref edge from subject to object. Idempotent.
func (rg *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	rg.writeMu.Lock()
	defer rg.writeMu.Unlock()
	ctx = disableStoreTracking(ctx)
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref")
	defer task.End()
	trace.Log(ctx, "hydra/block-gc/refgraph/add-ref/shape", "edges=1")

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref/build-quad")
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	subtask.End()

	taskCtx, subtask = trace.NewTask(taskCtx, "hydra/block-gc/refgraph/add-ref/add-quad")
	err := rg.handle.AddQuad(taskCtx, q)
	subtask.End()
	return err
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (rg *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	rg.writeMu.Lock()
	defer rg.writeMu.Unlock()
	ctx = disableStoreTracking(ctx)
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	return rg.handle.RemoveQuad(ctx, q)
}

// ApplyRefBatch serializes one bounded ownership transition under the RefGraph
// owner lock. It applies additions before removals, treats missing exact
// removals as no-ops, and derives orphan marks from the resulting owner set.
// Preparation and application are bounded together so each slice commits
// before preparation of the next slice begins.
func (rg *RefGraph) ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error {
	return rg.applyRefBatch(ctx, adds, removes, true, true)
}

func (rg *RefGraph) applyRefBatch(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
	lockPerSlice bool,
) error {
	ctx = disableStoreTracking(ctx)
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/apply-ref-batch")
	defer task.End()
	trace.Logf(
		ctx,
		"hydra/block-gc/refgraph/apply-ref-batch/shape",
		"adds=%d removes=%d slice_limit=%d transaction_limit=%d",
		len(adds),
		len(removes),
		refGraphApplySliceLimit,
		refGraphApplyBatchLimit,
	)

	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}
	slice := 0
	for len(adds) != 0 || len(removes) != 0 {
		if err := ctx.Err(); err != nil {
			return &refBatchError{
				err:     err,
				adds:    cloneRefEdges(adds),
				removes: cloneRefEdges(removes),
			}
		}

		addCount, removeCount := refBatchSliceCounts(adds, removes)
		slice++
		trace.Logf(
			ctx,
			"hydra/block-gc/refgraph/apply-ref-batch/slice-start",
			"slice=%d adds=%d removes=%d remaining_adds=%d remaining_removes=%d",
			slice,
			addCount,
			removeCount,
			len(adds)-addCount,
			len(removes)-removeCount,
		)

		sliceAdds := adds[:addCount]
		sliceRemoves := removes[:removeCount]
		if lockPerSlice {
			rg.writeMu.Lock()
		}
		preparedAdds, preparedRemoves, err := rg.prepareRefBatch(
			ctx,
			sliceAdds,
			sliceRemoves,
			markOrphaned,
		)
		if err == nil {
			var remainingAdds, remainingRemoves []RefEdge
			remainingAdds, remainingRemoves, err = rg.applyRefBatchSliceLocked(
				ctx,
				preparedAdds,
				preparedRemoves,
			)
			if err != nil {
				if lockPerSlice {
					rg.writeMu.Unlock()
				}
				return &refBatchError{
					err:     err,
					adds:    appendRefEdges(remainingAdds, adds[addCount:]),
					removes: appendRefEdges(remainingRemoves, removes[removeCount:]),
				}
			}
		}
		if lockPerSlice {
			rg.writeMu.Unlock()
		}
		if err != nil {
			return &refBatchError{
				err:     err,
				adds:    appendRefEdges(sliceAdds, adds[addCount:]),
				removes: appendRefEdges(sliceRemoves, removes[removeCount:]),
			}
		}
		trace.Logf(
			ctx,
			"hydra/block-gc/refgraph/apply-ref-batch/slice-complete",
			"slice=%d adds=%d removes=%d",
			slice,
			addCount,
			removeCount,
		)
		adds = adds[addCount:]
		removes = removes[removeCount:]
	}
	trace.Logf(ctx, "hydra/block-gc/refgraph/apply-ref-batch/slices", "slices=%d", slice)
	return nil
}

func refBatchSliceCounts(adds, removes []RefEdge) (int, int) {
	addCount := min(len(adds), refGraphApplySliceLimit)
	removeCount := 0
	if addCount < refGraphApplySliceLimit {
		removeCount = min(len(removes), refGraphApplySliceLimit-addCount)
	}
	if addCount == 0 {
		removeCount = min(len(removes), refGraphApplySliceLimit)
	}
	return addCount, removeCount
}

func cloneRefEdges(edges []RefEdge) []RefEdge {
	return append([]RefEdge(nil), edges...)
}

func appendRefEdges(first, second []RefEdge) []RefEdge {
	if len(first) == 0 {
		return cloneRefEdges(second)
	}
	if len(second) == 0 {
		return cloneRefEdges(first)
	}
	out := make([]RefEdge, 0, len(first)+len(second))
	out = append(out, first...)
	out = append(out, second...)
	return out
}

func (rg *RefGraph) applyRefBatchSliceLocked(
	ctx context.Context,
	adds, removes []RefEdge,
) ([]RefEdge, []RefEdge, error) {
	chunks := 0
	for len(adds) != 0 {
		count := min(len(adds), refGraphApplyBatchLimit)
		chunks++
		if err := rg.applyRefBatchChunk(ctx, adds[:count], nil); err != nil {
			return adds, removes, err
		}
		adds = adds[count:]
	}
	for len(removes) != 0 {
		count := min(len(removes), refGraphApplyBatchLimit)
		chunks++
		if err := rg.applyRefBatchChunk(ctx, nil, removes[:count]); err != nil {
			return adds, removes, err
		}
		removes = removes[count:]
	}
	trace.Logf(ctx, "hydra/block-gc/refgraph/apply-ref-batch/chunks", "chunks=%d", chunks)
	return nil, nil, nil
}

func (rg *RefGraph) applyRefBatchChunk(ctx context.Context, adds, removes []RefEdge) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/apply-ref-batch/apply-transaction")
	defer task.End()
	trace.Logf(ctx, "hydra/block-gc/refgraph/apply-ref-batch/apply-transaction/shape", "adds=%d removes=%d", len(adds), len(removes))

	n := len(adds) + len(removes)
	tx := graph.NewTransactionN(n)
	for _, e := range adds {
		tx.AddQuad(quad.Make(quad.IRI(e.Subject), quad.IRI(PredGCRef), quad.IRI(e.Object), nil))
	}
	for _, e := range removes {
		tx.RemoveQuad(quad.Make(quad.IRI(e.Subject), quad.IRI(PredGCRef), quad.IRI(e.Object), nil))
	}
	return rg.handle.ApplyTransaction(ctx, tx)
}

func (rg *RefGraph) prepareRefBatch(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
) ([]RefEdge, []RefEdge, error) {
	if _, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore); ok {
		return rg.prepareNativeRefBatch(ctx, adds, removes, markOrphaned)
	}

	removes, err := rg.filterExistingRemoves(ctx, adds, removes)
	if err != nil {
		return nil, nil, err
	}
	return rg.prepareOrphanMarks(ctx, adds, removes, markOrphaned)
}

func (rg *RefGraph) prepareNativeRefBatch(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
) ([]RefEdge, []RefEdge, error) {
	if !markOrphaned || len(removes) == 0 {
		return adds, removes, nil
	}

	type ownerState struct {
		owners  map[string]struct{}
		hadEdge bool
	}
	owners := make(map[string]ownerState)
	stagingRemoves := make(map[string]struct{})
	for _, edge := range removes {
		if edge.Subject == NodeUnreferenced {
			stagingRemoves[edge.Object] = struct{}{}
			continue
		}
		if IsPermanentRoot(edge.Object) {
			continue
		}
		if _, ok := owners[edge.Object]; ok {
			continue
		}
		sources, err := rg.GetIncomingRefs(ctx, edge.Object)
		if err != nil {
			return nil, nil, err
		}
		set := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			if source != NodeUnreferenced {
				set[source] = struct{}{}
			}
		}
		owners[edge.Object] = ownerState{owners: set}
	}

	for _, edge := range adds {
		if state, ok := owners[edge.Object]; ok && edge.Subject != NodeUnreferenced {
			state.owners[edge.Subject] = struct{}{}
			owners[edge.Object] = state
		}
	}
	for _, edge := range removes {
		if state, ok := owners[edge.Object]; ok {
			if _, exists := state.owners[edge.Subject]; exists {
				state.hadEdge = true
				delete(state.owners, edge.Subject)
				owners[edge.Object] = state
			}
		}
	}
	for object, state := range owners {
		if !state.hadEdge || len(state.owners) != 0 {
			continue
		}
		if _, removingStaging := stagingRemoves[object]; removingStaging {
			continue
		}
		adds = append(adds, RefEdge{Subject: NodeUnreferenced, Object: object})
	}
	return adds, removes, nil
}

func (rg *RefGraph) prepareOrphanMarks(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
) ([]RefEdge, []RefEdge, error) {
	if !markOrphaned || len(removes) == 0 {
		return adds, removes, nil
	}

	owners := make(map[string]map[string]struct{})
	stagingRemoves := make(map[string]struct{})
	for _, edge := range removes {
		if edge.Subject == NodeUnreferenced {
			stagingRemoves[edge.Object] = struct{}{}
			continue
		}
		if IsPermanentRoot(edge.Object) {
			continue
		}
		if _, ok := owners[edge.Object]; ok {
			continue
		}
		sources, err := rg.GetIncomingRefs(ctx, edge.Object)
		if err != nil {
			return nil, nil, err
		}
		set := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			if source != NodeUnreferenced {
				set[source] = struct{}{}
			}
		}
		owners[edge.Object] = set
	}
	for _, edge := range removes {
		if set, ok := owners[edge.Object]; ok {
			delete(set, edge.Subject)
		}
	}
	for _, edge := range adds {
		if set, ok := owners[edge.Object]; ok && edge.Subject != NodeUnreferenced {
			set[edge.Subject] = struct{}{}
		}
	}
	for object, set := range owners {
		if len(set) != 0 {
			continue
		}
		if _, removingStaging := stagingRemoves[object]; removingStaging {
			continue
		}
		adds = append(adds, RefEdge{Subject: NodeUnreferenced, Object: object})
	}
	return adds, removes, nil
}

func (rg *RefGraph) filterExistingRemoves(
	ctx context.Context,
	adds, removes []RefEdge,
) ([]RefEdge, error) {
	added := make(map[RefEdge]struct{}, len(adds))
	for _, edge := range adds {
		added[edge] = struct{}{}
	}
	if qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore); ok {
		return rg.filterExistingRemovesIndexed(ctx, added, removes, qs)
	}

	existing := make([]RefEdge, 0, len(removes))
	for _, edge := range removes {
		if _, ok := added[edge]; ok {
			existing = append(existing, edge)
			continue
		}
		found, err := rg.hasRefGeneric(ctx, edge.Subject, edge.Object)
		if err != nil {
			return nil, err
		}
		if found {
			existing = append(existing, edge)
		}
	}
	return existing, nil
}

func (rg *RefGraph) hasRefGeneric(ctx context.Context, subject, object string) (bool, error) {
	var found bool
	err := iterateFilteredNodeRefs(ctx, rg.handle, quad.Quad{
		Subject:   quad.IRI(subject),
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(object),
	}, quad.Subject, func(graph.Ref) error {
		found = true
		return io.EOF
	})
	return found, err
}

func (rg *RefGraph) filterExistingRemovesIndexed(
	ctx context.Context,
	added map[RefEdge]struct{},
	removes []RefEdge,
	qs *cayley_kv.QuadStore,
) ([]RefEdge, error) {
	existing := make([]RefEdge, 0, len(removes))
	lookup := make([]string, 1, 1+2*len(removes))
	lookup[0] = PredGCRef
	lookupSet := map[string]struct{}{PredGCRef: {}}
	unadded := 0
	for _, edge := range removes {
		if _, ok := added[edge]; ok {
			continue
		}
		unadded++
		if _, ok := lookupSet[edge.Subject]; !ok {
			lookupSet[edge.Subject] = struct{}{}
			lookup = append(lookup, edge.Subject)
		}
		if _, ok := lookupSet[edge.Object]; !ok {
			lookupSet[edge.Object] = struct{}{}
			lookup = append(lookup, edge.Object)
		}
	}
	if unadded == 0 {
		return removes, nil
	}
	ids, err := resolveIRIRefIDs(ctx, qs, lookup)
	if err != nil {
		return nil, errors.Wrap(err, "resolve remove refs")
	}
	predID := ids[PredGCRef]
	if predID == 0 {
		for _, edge := range removes {
			if _, ok := added[edge]; ok {
				existing = append(existing, edge)
			}
		}
		return existing, nil
	}

	byObject := make(map[uint64]map[uint64][]int)
	for i, edge := range removes {
		if _, ok := added[edge]; ok {
			continue
		}
		subjectID := ids[edge.Subject]
		objectID := ids[edge.Object]
		if subjectID == 0 || objectID == 0 {
			continue
		}
		bySubject := byObject[objectID]
		if bySubject == nil {
			bySubject = make(map[uint64][]int)
			byObject[objectID] = bySubject
		}
		bySubject[subjectID] = append(bySubject[subjectID], i)
	}
	found := make([]bool, len(removes))
	for objectID, bySubject := range byObject {
		remaining := 0
		for _, indexes := range bySubject {
			remaining += len(indexes)
		}
		err := iterateIncomingIndexRefs(ctx, qs, objectID, predID,
			func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
				indexes, ok := bySubject[uint64(ref)]
				if !ok {
					return nil
				}
				live, err := hasLive()
				if err != nil {
					return err
				}
				if !live {
					return nil
				}
				for _, index := range indexes {
					if !found[index] {
						found[index] = true
						remaining--
					}
				}
				if remaining == 0 {
					return io.EOF
				}
				return nil
			},
		)
		if err != nil {
			return nil, errors.Wrap(err, "iterate remove object index")
		}
	}
	for i, edge := range removes {
		if _, ok := added[edge]; ok || found[i] {
			existing = append(existing, edge)
		}
	}
	return existing, nil
}

// RemoveNodeRefs removes ALL outgoing gc/ref edges for a node.
// Returns the list of target IRIs that lost an incoming edge.
// If markOrphaned is true, targets that have no remaining incoming
// refs (excluding from "unreferenced") get an unreferenced edge.
func (rg *RefGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	rg.writeMu.Lock()
	defer rg.writeMu.Unlock()

	targets, err := rg.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	removes := make([]RefEdge, 0, len(targets))
	for _, target := range targets {
		removes = append(removes, RefEdge{Subject: node, Object: target})
	}
	if err := rg.applyRefBatch(ctx, nil, removes, markOrphaned, false); err != nil {
		return nil, err
	}
	return targets, nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
// Excludes edges from "unreferenced" (those don't count as real refs).
func (rg *RefGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	return rg.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding checks if a node has any incoming gc/ref edges.
// Excludes edges from "unreferenced" and the specified source nodes.
func (rg *RefGraph) HasIncomingRefsExcluding(
	ctx context.Context,
	node string,
	excluded ...string,
) (bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/resolve-excluded")
	excludedIRIs := make([]string, 0, len(excluded)+1)
	excludedIRIs = append(excludedIRIs, NodeUnreferenced)
	excludedIRIs = append(excludedIRIs, excluded...)
	excludedSet, err := rg.resolveIRIRefKeys(taskCtx, excludedIRIs)
	if err != nil {
		subtask.End()
		return false, errors.Wrap(err, "resolve excluded refs")
	}
	subtask.End()

	var found bool
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/iterate-candidates")
	found, usedFast, err := rg.hasIncomingRefsExcludingFast(taskCtx, node, excludedSet)
	if err == nil && !usedFast {
		err = iterateFilteredNodeRefs(taskCtx, rg.handle, quad.Quad{
			Predicate: quad.IRI(PredGCRef),
			Object:    quad.IRI(node),
		}, quad.Subject, func(ref graph.Ref) error {
			if _, ok := excludedSet[refs.ToKey(ref)]; !ok {
				found = true
				return io.EOF
			}
			return nil
		})
	}
	subtask.End()
	return found, errors.Wrap(err, "iterate incoming candidates")
}

func (rg *RefGraph) resolveIRIRefKeys(ctx context.Context, iris []string) (map[any]struct{}, error) {
	excludedSet := make(map[any]struct{}, len(iris))
	toResolve := make([]quad.Value, 0, len(iris))
	toResolveIRIs := make([]string, 0, len(iris))

	rg.mu.Lock()
	for _, iri := range iris {
		if key, ok := rg.iriRefKeys[iri]; ok {
			excludedSet[key] = struct{}{}
			continue
		}
		toResolveIRIs = append(toResolveIRIs, iri)
		toResolve = append(toResolve, quad.IRI(iri))
	}
	rg.mu.Unlock()

	if len(toResolve) == 0 {
		return excludedSet, nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/resolve-excluded/refs-of")
	var (
		resolved []graph.Ref
		err      error
	)
	if bq, ok := rg.handle.QuadStore.(refs.BatchNamer); ok {
		resolved, err = bq.RefsOf(taskCtx, toResolve)
	} else {
		resolved = make([]graph.Ref, len(toResolve))
		for i, node := range toResolve {
			resolved[i], err = rg.handle.ValueOf(taskCtx, node)
			if err != nil {
				break
			}
		}
	}
	subtask.End()
	if err != nil {
		return nil, err
	}

	_, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/resolve-excluded/cache-refs")
	rg.mu.Lock()
	for i, ref := range resolved {
		if ref == nil {
			continue
		}
		key := refs.ToKey(ref)
		iri := toResolveIRIs[i]
		if rg.iriRefKeys == nil {
			rg.iriRefKeys = make(map[string]any)
		}
		rg.iriRefKeys[iri] = key
		excludedSet[key] = struct{}{}
	}
	rg.mu.Unlock()
	subtask.End()

	return excludedSet, nil
}

// CloneIRIRefKeys returns a snapshot of the positive IRI ref-key cache.
func (rg *RefGraph) CloneIRIRefKeys() map[string]any {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	if len(rg.iriRefKeys) == 0 {
		return nil
	}
	out := make(map[string]any, len(rg.iriRefKeys))
	maps.Copy(out, rg.iriRefKeys)
	return out
}

// ImportIRIRefKeys seeds the positive IRI ref-key cache.
func (rg *RefGraph) ImportIRIRefKeys(keys map[string]any) {
	if len(keys) == 0 {
		return
	}

	rg.mu.Lock()
	defer rg.mu.Unlock()

	if rg.iriRefKeys == nil {
		rg.iriRefKeys = make(map[string]any, len(keys))
	}
	maps.Copy(rg.iriRefKeys, keys)
}

// GetOutgoingRefs returns all targets of gc/ref edges from the given node.
func (rg *RefGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/get-outgoing-refs")
	defer task.End()

	subjRef, err := rg.handle.ValueOf(ctx, quad.IRI(node))
	if err != nil || subjRef == nil {
		return nil, errors.Wrap(err, "lookup outgoing subject")
	}
	predRef, err := rg.handle.ValueOf(ctx, quad.IRI(PredGCRef))
	if err != nil || predRef == nil {
		return nil, errors.Wrap(err, "lookup gc/ref predicate")
	}
	predKey := refs.ToKey(predRef)

	it := rg.handle.QuadIterator(ctx, quad.Subject, subjRef).Iterate(ctx)
	defer it.Close()

	var nodeRefs []graph.Ref
	for {
		if !it.Next(ctx) {
			if err := it.Err(); err != nil {
				return nil, errors.Wrap(err, "iterate outgoing subject index")
			}
			return resolveNodeIRIs(ctx, rg.handle, nodeRefs)
		}
		quadRef, err := it.Result(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "read outgoing quad ref")
		}
		gotPred, err := rg.handle.QuadDirection(ctx, quadRef, quad.Predicate)
		if err != nil {
			return nil, errors.Wrap(err, "read outgoing predicate")
		}
		if refs.ToKey(gotPred) != predKey {
			continue
		}
		objRef, err := rg.handle.QuadDirection(ctx, quadRef, quad.Object)
		if err != nil {
			return nil, errors.Wrap(err, "read outgoing object")
		}
		nodeRefs = append(nodeRefs, objRef)
	}
}

// GetIncomingRefs returns all sources that have gc/ref edges pointing to the given node.
func (rg *RefGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/get-incoming-refs")
	defer task.End()

	if qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore); ok {
		ids, err := resolveIRIRefIDs(ctx, qs, []string{PredGCRef, node})
		if err != nil {
			return nil, errors.Wrap(err, "resolve incoming refs")
		}
		predID := ids[PredGCRef]
		objectID := ids[node]
		if predID == 0 || objectID == 0 {
			return nil, nil
		}

		var nodeRefs []graph.Ref
		err = iterateIncomingIndexRefs(ctx, qs, objectID, predID,
			func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
				live, err := hasLive()
				if err != nil {
					return err
				}
				if live {
					nodeRefs = append(nodeRefs, ref)
				}
				return nil
			},
		)
		if err != nil {
			return nil, errors.Wrap(err, "iterate incoming object index")
		}
		return resolveNodeIRIs(ctx, rg.handle, nodeRefs)
	}

	return collectFilteredNodeIRIs(ctx, rg.handle, quad.Quad{
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(node),
	}, quad.Subject)
}

// GetUnreferencedNodes returns all nodes that have a gc/ref from "unreferenced".
func (rg *RefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return rg.GetOutgoingRefs(ctx, NodeUnreferenced)
}

// Close closes the underlying graph handle.
func (rg *RefGraph) Close() error {
	return rg.handle.Close()
}

// AddBlockRef adds gc/ref from source block to target block.
func (rg *RefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	s := BlockIRI(source)
	t := BlockIRI(target)
	if s == "" || t == "" {
		return nil
	}
	return rg.AddRef(ctx, s, t)
}

// AddObjectRoot adds gc/ref from object:{key} to block.
func (rg *RefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := BlockIRI(ref)
	if t == "" {
		return nil
	}
	return rg.AddRef(ctx, ObjectIRI(objectKey), t)
}

// RemoveObjectRoot removes gc/ref from object:{key} to block.
func (rg *RefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := BlockIRI(ref)
	if t == "" {
		return nil
	}
	return rg.RemoveRef(ctx, ObjectIRI(objectKey), t)
}

// buildQuadFilters builds quad filters for the non-empty directions in gq.
func buildQuadFilters(gq quad.Quad) shape.Quads {
	var q shape.Quads
	if gq.Subject != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Subject, Values: shape.Lookup([]quad.Value{gq.Subject})})
	}
	if gq.Predicate != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Predicate, Values: shape.Lookup([]quad.Value{gq.Predicate})})
	}
	if gq.Object != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Object, Values: shape.Lookup([]quad.Value{gq.Object})})
	}
	if gq.Label != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Label, Values: shape.Lookup([]quad.Value{gq.Label})})
	}
	return q
}

func (rg *RefGraph) hasIncomingRefsExcludingFast(
	ctx context.Context,
	node string,
	excludedSet map[any]struct{},
) (bool, bool, error) {
	qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore)
	if !ok {
		return false, false, nil
	}
	ids, err := resolveIRIRefIDs(ctx, qs, []string{PredGCRef, node})
	if err != nil {
		return false, true, errors.Wrap(err, "lookup incoming refs")
	}
	predID := ids[PredGCRef]
	objID := ids[node]
	if predID == 0 || objID == 0 {
		return false, true, nil
	}

	var found bool
	err = iterateIncomingIndexRefs(ctx, qs, objID, predID,
		func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
			if _, ok := excludedSet[refs.ToKey(ref)]; ok {
				return nil
			}
			live, err := hasLive()
			if err != nil {
				return err
			}
			if !live {
				return nil
			}
			found = true
			return io.EOF
		},
	)
	return found, true, errors.Wrap(err, "iterate incoming object index")
}

func resolveIRIRefIDs(
	ctx context.Context,
	qs *cayley_kv.QuadStore,
	iris []string,
) (map[string]uint64, error) {
	values := make([]quad.Value, len(iris))
	for i, iri := range iris {
		values[i] = quad.IRI(iri)
	}
	refs, err := qs.RefsOf(ctx, values)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]uint64, len(iris))
	for i, ref := range refs {
		id, ok := ref.(cayley_kv.Int64Value)
		if ok && id != 0 {
			ids[iris[i]] = uint64(id)
		}
	}
	return ids, nil
}

func iterateIncomingIndexRefs(
	ctx context.Context,
	qs *cayley_kv.QuadStore,
	objectID, predID uint64,
	cb func(cayley_kv.Int64Value, func() (bool, error)) error,
) error {
	return qs.IterateIndexPrefixNextRefs(
		ctx,
		cayley_kv.DefaultQuadIndexes[1],
		[]uint64{objectID, predID},
		cb,
	)
}

// iterateFilteredNodeRefs iterates node refs on dir from quads matching gq.
func iterateFilteredNodeRefs(
	ctx context.Context,
	h *cayley.Handle,
	gq quad.Quad,
	dir quad.Direction,
	cb func(ref graph.Ref) error,
) error {
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/optimize-shape")
	sh, _, err := shape.Optimize(taskCtx, shape.NodesFrom{
		Dir:   dir,
		Quads: buildQuadFilters(gq),
	}, h)
	subtask.End()
	if err != nil {
		return err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/build-iterator")
	it := sh.BuildIterator(taskCtx, h).Iterate(taskCtx)
	subtask.End()
	defer it.Close()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/iterate")
	defer subtask.End()
	for {
		if !it.Next(taskCtx) {
			if err := it.Err(); err != nil {
				return err
			}
			return nil
		}
		ref, err := it.Result(taskCtx)
		if err != nil {
			return err
		}
		if err := cb(ref); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// collectFilteredNodeIRIs collects node IRIs on dir from quads matching gq.
func collectFilteredNodeIRIs(
	ctx context.Context,
	h *cayley.Handle,
	gq quad.Quad,
	dir quad.Direction,
) ([]string, error) {
	var nodeRefs []graph.Ref
	if err := iterateFilteredNodeRefs(ctx, h, gq, dir, func(ref graph.Ref) error {
		nodeRefs = append(nodeRefs, ref)
		return nil
	}); err != nil {
		return nil, err
	}
	if len(nodeRefs) == 0 {
		return nil, nil
	}
	return resolveNodeIRIs(ctx, h, nodeRefs)
}

func resolveNodeIRIs(ctx context.Context, h *cayley.Handle, nodeRefs []graph.Ref) ([]string, error) {
	vals, err := graph.ValuesOf(ctx, h, nodeRefs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, iriString(v))
	}
	return out, nil
}

// iriString extracts the string value from a quad.Value, assuming it is an IRI.
func iriString(v quad.Value) string {
	if v == nil {
		return ""
	}
	iri, ok := v.(quad.IRI)
	if ok {
		return string(iri)
	}
	return ""
}

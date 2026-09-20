package block_gc

import (
	"context"
	"encoding/binary"
	"io"
	"slices"
	"strconv"
	"sync"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/graph/refs"
	cayley_hkv "github.com/aperturerobotics/cayley/kv"
	cayley_flat "github.com/aperturerobotics/cayley/kv/flat"
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
// The durable graph is the only representation of the edge set. Membership and
// owner queries read the store's own indexes, so opening a RefGraph costs
// nothing proportional to how many edges it holds and neither does keeping one
// open.
type RefGraph struct {
	// handle owns the Cayley indexes and their cached metadata.
	handle *cayley.Handle
	// store addresses the same prefixed durable graph as handle.
	store kvtx.Store

	// writeMu serializes bounded ownership transitions.
	writeMu sync.Mutex
}

const (
	// refGraphApplySliceLimit bounds preparation and application together.
	refGraphApplySliceLimit = 4096
	// Commit each bounded ownership slice without subdividing it into extra
	// fsyncs. Additions still commit before removals, and slices release writeMu.
	refGraphApplyBatchLimit = refGraphApplySliceLimit
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

func (e *refBatchError) RefBatchRemainder() ([]RefEdge, []RefEdge) {
	return e.adds, e.removes
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
	return &RefGraph{handle: h, store: prefixed}, nil
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
	found, err := rg.hasRef(ctx, subject, object)
	if err != nil {
		return errors.Wrap(err, "check existing ref edge")
	}
	if found {
		return nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref/build-quad")
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	subtask.End()

	taskCtx, subtask = trace.NewTask(taskCtx, "hydra/block-gc/refgraph/add-ref/add-quad")
	err = rg.handle.AddQuad(taskCtx, q)
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

// ApplyRefBatch serializes one bounded ownership transition under writeMu. It
// applies additions before removals, treats missing exact removals as no-ops,
// and derives orphan marks from the resulting owner set.
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
	// Disable nested store tracking before tracing the bounded batch.
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

	// Return early when the batch carries no ownership transition.
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

		// Slice the transition and acquire the write lock for each slice.
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

		// Prepare durable additions and removals before applying the slice.
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

		// Advance to the next slice after recording its successful application.
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
	return slices.Clone(edges)
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
	// Commit additions in bounded chunks before committing removals.
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

	// Materialize one transaction preserving additions-before-removals order.
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
	// Remove idempotent changes before deriving orphan markers.
	adds, removes, err := rg.filterRefChanges(ctx, adds, removes)
	if err != nil {
		return nil, nil, err
	}
	return rg.prepareOrphanMarks(ctx, adds, removes, markOrphaned)
}

func (rg *RefGraph) prepareOrphanMarks(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
) ([]RefEdge, []RefEdge, error) {
	if !markOrphaned || len(removes) == 0 {
		return adds, removes, nil
	}

	// Only removed owners matter to the existence query. Loading every current
	// owner makes releasing one reference proportional to a block's sharing.
	removedOwners := make(map[string]map[string]struct{})
	for _, edge := range removes {
		if IsPermanentRoot(edge.Object) {
			continue
		}
		set := removedOwners[edge.Object]
		if set == nil {
			set = make(map[string]struct{})
			removedOwners[edge.Object] = set
		}
		set[edge.Subject] = struct{}{}
	}

	// A new owner survives unless the same batch removes it. Additions land
	// first, so an edge present in both lists cannot keep its target alive.
	for _, edge := range adds {
		if set, ok := removedOwners[edge.Object]; ok && edge.Subject != NodeUnreferenced {
			if _, removed := set[edge.Subject]; !removed {
				delete(removedOwners, edge.Object)
			}
		}
	}

	// An explicit staging removal must not recreate its marker. Otherwise,
	// stop the indexed lookup as soon as one unaffected owner is found.
	for object, set := range removedOwners {
		if _, removingStaging := set[NodeUnreferenced]; removingStaging {
			continue
		}
		excluded := make([]string, 0, len(set))
		for owner := range set {
			excluded = append(excluded, owner)
		}
		owned, err := rg.HasIncomingRefsExcluding(ctx, object, excluded...)
		if err != nil {
			return nil, nil, err
		}
		if !owned {
			adds = append(adds, RefEdge{Subject: NodeUnreferenced, Object: object})
		}
	}
	return adds, removes, nil
}

// filterRefChanges checks exact membership once for the whole slice. Existing
// additions and absent removals need no write. An edge added by this batch exists
// for a following removal even when it was absent in the durable graph.
func (rg *RefGraph) filterRefChanges(ctx context.Context, adds, removes []RefEdge) ([]RefEdge, []RefEdge, error) {
	added := make(map[RefEdge]struct{}, len(adds))
	probes := make([]RefEdge, 0, len(adds)+len(removes))
	probes = append(probes, adds...)
	for _, edge := range adds {
		added[edge] = struct{}{}
	}
	for _, edge := range removes {
		if _, ok := added[edge]; !ok {
			probes = append(probes, edge)
		}
	}
	found, err := rg.hasRefs(ctx, probes)
	if err != nil {
		return nil, nil, err
	}

	// Preserve caller order without modifying retry inputs.
	newAdds := make([]RefEdge, 0, len(adds))
	for i, edge := range adds {
		if !found[i] {
			newAdds = append(newAdds, edge)
		}
	}
	existingRemoves := make([]RefEdge, 0, len(removes))
	index := len(adds)
	for _, edge := range removes {
		present := true
		if _, ok := added[edge]; !ok {
			present = found[index]
			index++
		}
		if present {
			existingRemoves = append(existingRemoves, edge)
		}
	}
	trace.Logf(ctx, "hydra/block-gc/refgraph/filter-changes", "adds=%d new_adds=%d removes=%d existing_removes=%d", len(adds), len(newAdds), len(removes), len(existingRemoves))
	return newAdds, existingRemoves, nil
}

// hasRef reports whether the durable graph holds the exact gc/ref edge. It
// reads the complete object-predicate-subject posting key rather than scanning
// keys that share its unpadded base62 prefix.
func (rg *RefGraph) hasRef(ctx context.Context, subject, object string) (bool, error) {
	found, err := rg.hasRefs(ctx, []RefEdge{{Subject: subject, Object: object}})
	if err != nil {
		return false, err
	}
	return found[0], nil
}

// hasRefs resolves node IDs in one batch and reads exact edge postings under
// one storage transaction. A stale snapshot retries the complete lookup.
func (rg *RefGraph) hasRefs(ctx context.Context, edges []RefEdge) ([]bool, error) {
	if len(edges) == 0 {
		return nil, ctx.Err()
	}
	var found []bool
	err := kvtx.RunOperation(ctx, func(ctx context.Context) error {
		var err error
		found, err = rg.hasRefsAttempt(ctx, edges)
		return err
	})
	return found, err
}

// hasRefsAttempt reads exact edges without external effects.
func (rg *RefGraph) hasRefsAttempt(ctx context.Context, edges []RefEdge) ([]bool, error) {
	found := make([]bool, len(edges))
	qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore)
	if !ok {
		for i, edge := range edges {
			var err error
			found[i], err = rg.hasRefGeneric(ctx, edge.Subject, edge.Object)
			if err != nil {
				return nil, err
			}
		}
		return found, nil
	}
	names := []string{PredGCRef}
	seen := map[string]struct{}{PredGCRef: {}}
	for _, edge := range edges {
		for _, name := range [2]string{edge.Subject, edge.Object} {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	ids, err := resolveIRIRefIDs(ctx, qs, names)
	if err != nil {
		return nil, errors.Wrap(err, "resolve exact ref edges")
	}
	predID := ids[PredGCRef]
	if predID == 0 {
		return found, nil
	}
	tx, err := rg.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open exact ref edge transaction")
	}
	defer tx.Discard()
	for i, edge := range edges {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		objectID, subjectID := ids[edge.Object], ids[edge.Subject]
		if objectID == 0 || subjectID == 0 {
			continue
		}
		found[i], err = hasRefInTransaction(ctx, tx, predID, objectID, subjectID)
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

// hasRefInTransaction checks the complete posting key and validates its primitive.
func hasRefInTransaction(ctx context.Context, tx kvtx.Tx, predID, objectID, subjectID uint64) (bool, error) {
	indexKey := cayley_flat.KeyEscape(cayley_kv.DefaultQuadIndexes[1].Key(
		[]uint64{objectID, predID, subjectID},
	))
	postings, found, err := tx.Get(ctx, indexKey)
	if err != nil {
		return false, errors.Wrap(err, "read exact ref edge index")
	}
	if !found {
		return false, nil
	}
	quadIDs := make([]uint64, 0, 1)
	for len(postings) != 0 {
		quadID, n := binary.Uvarint(postings)
		if n <= 0 {
			return false, errors.New("decode exact ref edge index")
		}
		quadIDs = append(quadIDs, quadID)
		postings = postings[n:]
	}
	for _, quadID := range slices.Backward(quadIDs) {

		logKey := cayley_flat.KeyEscape(cayley_hkv.Key{
			[]byte("l"),
			[]byte(strconv.FormatUint(quadID, 10)),
		})
		data, found, err := tx.Get(ctx, logKey)
		if err != nil {
			return false, errors.Wrap(err, "read exact ref edge primitive")
		}
		if !found {
			return false, errors.Errorf("exact ref edge primitive %d is missing", quadID)
		}
		var prim cayley_proto.Primitive
		if err := prim.UnmarshalVT(data); err != nil {
			return false, errors.Wrap(err, "decode exact ref edge primitive")
		}
		if !prim.Deleted &&
			prim.Object == objectID &&
			prim.Predicate == predID &&
			prim.Subject == subjectID &&
			prim.Label == 0 {
			return true, nil
		}
	}
	return false, nil
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

	for _, iri := range iris {
		toResolve = append(toResolve, quad.IRI(iri))
	}

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

	for _, ref := range resolved {
		if ref == nil {
			continue
		}
		excludedSet[refs.ToKey(ref)] = struct{}{}
	}

	return excludedSet, nil
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

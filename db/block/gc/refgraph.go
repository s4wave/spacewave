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
	// refGraphApplyBatchLimit commits each bounded slice without extra
	// fsyncs. Additions precede removals within and across commits.
	// Each slice releases writeMu.
	refGraphApplyBatchLimit = refGraphApplySliceLimit
)

// NewRefGraph constructs a RefGraph backed by the given kvtx store.
// prefix is prepended to all keys (e.g., "gc/" for space context).
func NewRefGraph(ctx context.Context, store kvtx.Store, prefix []byte) (*RefGraph, error) {
	// Open the prefixed Cayley graph with idempotent edge updates.
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
	// Require enough nodes to define a chain.
	if len(nodes) < 2 {
		return errors.New("RegisterEntityChain requires at least 2 nodes")
	}

	// Register every adjacent pair in chain order.
	for i := 0; i < len(nodes)-1; i++ {
		if err := rg.AddRef(ctx, nodes[i], nodes[i+1]); err != nil {
			return err
		}
	}
	return nil
}

// AddRef adds a gc/ref edge from subject to object. Idempotent.
func (rg *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	// Serialize the RefGraph edge addition and trace its storage work.
	rg.writeMu.Lock()
	defer rg.writeMu.Unlock()
	ctx = disableStoreTracking(ctx)
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref")
	defer task.End()
	trace.Log(ctx, "hydra/block-gc/refgraph/add-ref/shape", "edges=1")

	// Skip the RefGraph write when the exact edge already exists.
	found, err := rg.hasRef(ctx, subject, object)
	if err != nil {
		return errors.Wrap(err, "check existing ref edge")
	}
	if found {
		return nil
	}

	// Construct the GC reference quad for the new edge.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref/build-quad")
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	subtask.End()

	// Apply the new quad to the durable RefGraph.
	taskCtx, subtask = trace.NewTask(taskCtx, "hydra/block-gc/refgraph/add-ref/add-quad")
	err = rg.handle.AddQuad(taskCtx, q)
	subtask.End()
	return err
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (rg *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	// Serialize removal of the exact GC reference quad.
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

// applyRefBatch commits bounded add-before-remove slices with optional orphan marks.
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

// refBatchSliceCounts fills one ownership slice with additions before removals.
func refBatchSliceCounts(adds, removes []RefEdge) (int, int) {
	// Fill the bounded RefGraph slice with additions before removals.
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

// cloneRefEdges returns an independent copy of an ownership change list.
func cloneRefEdges(edges []RefEdge) []RefEdge {
	return slices.Clone(edges)
}

// appendRefEdges copies two ownership change lists into one.
func appendRefEdges(first, second []RefEdge) []RefEdge {
	// Reuse a copy of the populated edge list when its counterpart is empty.
	if len(first) == 0 {
		return cloneRefEdges(second)
	}
	if len(second) == 0 {
		return cloneRefEdges(first)
	}

	// Combine both edge lists without retaining their backing arrays.
	out := make([]RefEdge, 0, len(first)+len(second))
	out = append(out, first...)
	out = append(out, second...)
	return out
}

// applyRefBatchSliceLocked commits a prepared slice and returns its uncommitted suffix.
func (rg *RefGraph) applyRefBatchSliceLocked(
	ctx context.Context,
	adds, removes []RefEdge,
) ([]RefEdge, []RefEdge, error) {
	// Derive orphan markers before removing opposing input edges.
	adds, removes = normalizeRefEdges(adds, removes)

	// Keep additions before removals within each bounded atomic commit.
	chunks := 0
	for len(adds) != 0 || len(removes) != 0 {
		addCount := min(len(adds), refGraphApplyBatchLimit)
		removeCount := min(len(removes), refGraphApplyBatchLimit-addCount)
		chunks++
		if err := rg.applyRefBatchChunk(ctx, adds[:addCount], removes[:removeCount]); err != nil {
			return adds, removes, err
		}
		adds = adds[addCount:]
		removes = removes[removeCount:]
	}

	// Record the number of committed RefGraph chunks.
	trace.Logf(ctx, "hydra/block-gc/refgraph/apply-ref-batch/chunks", "chunks=%d", chunks)
	return nil, nil, nil
}

// applyRefBatchChunk applies one atomic add-before-remove graph transaction.
func (rg *RefGraph) applyRefBatchChunk(ctx context.Context, adds, removes []RefEdge) error {
	// Trace the durable transaction for this RefGraph chunk.
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/apply-ref-batch/apply-transaction")
	defer task.End()
	trace.Logf(ctx, "hydra/block-gc/refgraph/apply-ref-batch/apply-transaction/shape", "adds=%d removes=%d", len(adds), len(removes))

	// Materialize one transaction preserving additions-before-removals order.
	n := len(adds) + len(removes)
	deltas := make([]graph.Delta, 0, n)
	for _, e := range adds {
		deltas = append(deltas, graph.Delta{Quad: quad.MakeIRI(e.Subject, PredGCRef, e.Object, ""), Action: graph.Add})
	}
	for _, e := range removes {
		deltas = append(deltas, graph.Delta{Quad: quad.MakeIRI(e.Subject, PredGCRef, e.Object, ""), Action: graph.Delete})
	}
	return rg.handle.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{IgnoreDup: true, IgnoreMissing: true})
}

// prepareRefBatch filters idempotent changes and derives orphan markers.
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

// prepareOrphanMarks stages targets whose final owner is removed by this batch.
func (rg *RefGraph) prepareOrphanMarks(
	ctx context.Context,
	adds, removes []RefEdge,
	markOrphaned bool,
) ([]RefEdge, []RefEdge, error) {
	// Leave orphan markers unchanged when the batch requests no orphan check.
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
	// Probe additions and removals that are absent from the addition set.
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
	// Leave an empty lookup to the caller's cancellation state.
	if len(edges) == 0 {
		return nil, ctx.Err()
	}

	// Retry all edge reads together when the storage snapshot becomes stale.
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
	// Resolve exact edges through the generic graph when the indexed store is unavailable.
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

	// Collect the distinct GC predicate, subject, and object IRIs for one lookup.
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

	// Resolve the GC edge nodes and stop when the predicate is absent.
	ids, err := resolveIRIRefIDs(ctx, qs, names)
	if err != nil {
		return nil, errors.Wrap(err, "resolve exact ref edges")
	}
	predID := ids[PredGCRef]
	if predID == 0 {
		return found, nil
	}

	// Open one read transaction for all exact edge postings.
	tx, err := rg.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "open exact ref edge transaction")
	}
	defer tx.Discard()

	return hasRefsInTransaction(ctx, tx, predID, ids, edges)
}

// hasRefsInTransaction batches postings and their newest live primitive checks.
// Every read uses the same snapshot; results retain the caller's edge order.
func hasRefsInTransaction(ctx context.Context, tx kvtx.Tx, predID uint64, ids map[string]uint64, edges []RefEdge) ([]bool, error) {
	// Collect exact posting keys for edges whose endpoints exist.
	found := make([]bool, len(edges))
	keys := make([][]byte, 0, len(edges))
	indexes := make([]int, 0, len(edges))
	for i, edge := range edges {
		objectID, subjectID := ids[edge.Object], ids[edge.Subject]
		if objectID == 0 || subjectID == 0 {
			continue
		}
		keys = append(keys, cayley_flat.KeyEscape(cayley_kv.DefaultQuadIndexes[1].Key(
			[]uint64{objectID, predID, subjectID},
		)))
		indexes = append(indexes, i)
	}

	// Read each exact posting once through the backend's batch operation.
	postings, present, err := kvtx.GetBatch(ctx, tx, keys)
	if err != nil {
		return nil, errors.Wrap(err, "read exact ref edge indexes")
	}
	quadIDs := make([][]uint64, len(keys))
	for i, posting := range postings {
		if !present[i] {
			continue
		}
		for len(posting) != 0 {
			quadID, n := binary.Uvarint(posting)
			if n <= 0 {
				return nil, errors.New("decode exact ref edge index")
			}
			quadIDs[i] = append(quadIDs[i], quadID)
			posting = posting[n:]
		}
	}

	// Validate newest primitives together, probing older entries only for edges
	// whose newest primitive is deleted or does not match the exact edge.
	logKeys := make([][]byte, 0, len(keys))
	positions := make([]int, 0, len(keys))
	for {
		// Collect one unresolved primitive per edge for this read batch.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logKeys, positions = logKeys[:0], positions[:0]
		for i, pending := range quadIDs {
			if found[indexes[i]] || len(pending) == 0 {
				continue
			}
			quadID := pending[len(pending)-1]
			logKeys = append(logKeys, cayley_flat.KeyEscape(cayley_hkv.Key{
				[]byte("l"), []byte(strconv.FormatUint(quadID, 10)),
			}))
			positions = append(positions, i)
		}
		if len(logKeys) == 0 {
			return found, nil
		}

		// Read the selected primitives in one backend batch.
		data, present, err := kvtx.GetBatch(ctx, tx, logKeys)
		if err != nil {
			return nil, errors.Wrap(err, "read exact ref edge primitives")
		}
		for j, i := range positions {
			// Require the indexed primitive and check its exact endpoints.
			pending := quadIDs[i]
			quadID := pending[len(pending)-1]
			quadIDs[i] = pending[:len(pending)-1]
			if !present[j] {
				return nil, errors.Errorf("exact ref edge primitive %d is missing", quadID)
			}
			var prim cayley_proto.Primitive
			if err := prim.UnmarshalVT(data[j]); err != nil {
				return nil, errors.Wrap(err, "decode exact ref edge primitive")
			}
			edge := edges[indexes[i]]
			found[indexes[i]] = !prim.Deleted &&
				prim.Object == ids[edge.Object] &&
				prim.Predicate == predID &&
				prim.Subject == ids[edge.Subject] && prim.Label == 0
		}
	}
}

// hasRefGeneric tests exact edge membership through the graph traversal API.
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
	// Serialize removal of the node ownership edges.
	rg.writeMu.Lock()
	defer rg.writeMu.Unlock()

	// Collect the node targets and remove their edges with the requested orphan marks.
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
	// Trace the RefGraph incoming-owner lookup.
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding")
	defer task.End()

	// Resolve staging and caller-excluded owners to graph reference keys.
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

	// Search the incoming index, falling back to a filtered graph traversal.
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

// resolveIRIRefKeys resolves present IRIs to comparable graph reference keys.
func (rg *RefGraph) resolveIRIRefKeys(ctx context.Context, iris []string) (map[any]struct{}, error) {
	// Allocate the excluded-owner set and its graph lookup values.
	excludedSet := make(map[any]struct{}, len(iris))
	toResolve := make([]quad.Value, 0, len(iris))

	// Convert the excluded owner IRIs into Cayley values.
	for _, iri := range iris {
		toResolve = append(toResolve, quad.IRI(iri))
	}

	// Avoid a graph lookup when no owner IRIs were supplied.
	if len(toResolve) == 0 {
		return excludedSet, nil
	}

	// Resolve excluded owners through the graph naming API.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/resolve-excluded/refs-of")
	var (
		resolved []graph.Ref
		err      error
	)
	switch qs := rg.handle.QuadStore.(type) {
	case refs.BatchNamer:
		resolved, err = qs.RefsOf(taskCtx, toResolve)
	default:
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

	// Retain keys only for excluded owners present in the graph.
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
	// Trace the RefGraph outgoing-edge lookup.
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/get-outgoing-refs")
	defer task.End()

	// Resolve the source and GC predicate before scanning outgoing quads.
	subjRef, err := rg.handle.ValueOf(ctx, quad.IRI(node))
	if err != nil || subjRef == nil {
		return nil, errors.Wrap(err, "lookup outgoing subject")
	}
	predRef, err := rg.handle.ValueOf(ctx, quad.IRI(PredGCRef))
	if err != nil || predRef == nil {
		return nil, errors.Wrap(err, "lookup gc/ref predicate")
	}
	predKey := refs.ToKey(predRef)

	// Open the outgoing subject iterator for the resolved source.
	it := rg.handle.QuadIterator(ctx, quad.Subject, subjRef).Iterate(ctx)
	defer it.Close()

	// Collect target references only from quads with the GC predicate.
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
	// Trace the incoming-edge lookup.
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/get-incoming-refs")
	defer task.End()

	// Traverse the incoming index when the graph exposes exact postings.
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

	// Fall back to the graph's filtered traversal for other stores.
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
	// Construct filters for each specified direction of the GC quad.
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

// hasIncomingRefsExcludingFast stops at the first indexed, live, unexcluded owner.
func (rg *RefGraph) hasIncomingRefsExcludingFast(
	ctx context.Context,
	node string,
	excludedSet map[any]struct{},
) (bool, bool, error) {
	// Use the incoming index only when the RefGraph has an indexed Cayley store.
	qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore)
	if !ok {
		return false, false, nil
	}

	// Resolve the GC predicate and target before searching incoming owners.
	ids, err := resolveIRIRefIDs(ctx, qs, []string{PredGCRef, node})
	if err != nil {
		return false, true, errors.Wrap(err, "lookup incoming refs")
	}
	predID := ids[PredGCRef]
	objID := ids[node]
	if predID == 0 || objID == 0 {
		return false, true, nil
	}

	// Stop the incoming-index scan at the first live, unexcluded owner.
	var found bool
	err = iterateIncomingIndexRefs(ctx, qs, objID, predID,
		func(ref cayley_kv.Int64Value, hasLive func() (bool, error)) error {
			// Discard excluded owners before checking their live postings.
			if _, ok := excludedSet[refs.ToKey(ref)]; ok {
				return nil
			}

			// Check whether the candidate owner has a live incoming edge.
			live, err := hasLive()
			if err != nil {
				return err
			}
			if !live {
				return nil
			}

			// Report the first surviving incoming owner.
			found = true
			return io.EOF
		},
	)
	return found, true, errors.Wrap(err, "iterate incoming object index")
}

// resolveIRIRefIDs resolves present IRIs to numeric graph node IDs.
func resolveIRIRefIDs(
	ctx context.Context,
	qs *cayley_kv.QuadStore,
	iris []string,
) (map[string]uint64, error) {
	// Convert and resolve the requested IRIs through the graph naming API.
	values := make([]quad.Value, len(iris))
	for i, iri := range iris {
		values[i] = quad.IRI(iri)
	}
	refs, err := qs.RefsOf(ctx, values)
	if err != nil {
		return nil, err
	}

	// Retain numeric IDs for nodes present in the indexed graph.
	ids := make(map[string]uint64, len(iris))
	for i, ref := range refs {
		id, ok := ref.(cayley_kv.Int64Value)
		if ok && id != 0 {
			ids[iris[i]] = uint64(id)
		}
	}
	return ids, nil
}

// iterateIncomingIndexRefs visits indexed sources and their live-edge checks.
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
	// Optimize the filtered GC quad traversal.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/optimize-shape")
	sh, _, err := shape.Optimize(taskCtx, shape.NodesFrom{
		Dir:   dir,
		Quads: buildQuadFilters(gq),
	}, h)
	subtask.End()
	if err != nil {
		return err
	}

	// Open the optimized graph iterator and retain its cleanup.
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/build-iterator")
	it := sh.BuildIterator(taskCtx, h).Iterate(taskCtx)
	subtask.End()
	defer it.Close()

	// Trace traversal and deliver each matching graph reference.
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
	// Collect matching graph references before resolving their node values.
	var nodeRefs []graph.Ref
	if err := iterateFilteredNodeRefs(ctx, h, gq, dir, func(ref graph.Ref) error {
		nodeRefs = append(nodeRefs, ref)
		return nil
	}); err != nil {
		return nil, err
	}

	// Resolve only the references collected by this traversal.
	if len(nodeRefs) == 0 {
		return nil, nil
	}
	return resolveNodeIRIs(ctx, h, nodeRefs)
}

// resolveNodeIRIs resolves graph references to IRI strings in traversal order.
func resolveNodeIRIs(ctx context.Context, h *cayley.Handle, nodeRefs []graph.Ref) ([]string, error) {
	// Resolve the collected graph nodes to IRI strings.
	vals, err := graph.ValuesOf(ctx, h, nodeRefs)
	if err != nil {
		return nil, err
	}

	// Collect the resolved IRI values in traversal order.
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

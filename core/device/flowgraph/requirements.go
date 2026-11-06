package device_flowgraph

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// claimRequirements streams the typed Flowgraphs and tries each claimable node.
// Each attempt is a short transaction; the read set watches candidates, Device
// labels and Worker capacity, including failed admission, without polling.
// Each watched revision scans typed graphs in bounded pages. Eligibility reads
// stay off the World writer; only matching candidates enter a claim transaction.
func (r *Reconciler) claimRequirements(ctx context.Context, reads *world.ReadSet, declaration *s4wave_device.ForgeWorkerDeclaration) error {
	// Keep orphan cleanup working even after the forge-worker node is removed.
	if err := r.releaseRemovedPlacements(ctx, reads); err != nil {
		return err
	}
	if declaration == nil {
		return nil
	}

	// Stream bounded pages from the type index while tracking every candidate body.
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	ws := reads.Track(tx)

	// Read eligibility once so mismatches and exhausted requests never take the writer.
	device, err := world.LookupObjectBody[s4wave_device.Device](ctx, ws, r.deviceKey)
	if err != nil {
		return err
	}
	labels := device.EffectiveLabels(r.deviceKey)
	policy := r.policy.Snapshot()

	// Follow the Worker claim and available capacity through the same watched read.
	capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, declaration.GetWorkerObjectKey())
	if errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		return nil
	}
	if err != nil {
		return err
	}
	if capacity.OwnerDeviceObjectKey != r.deviceKey || capacity.OwnerClaimActive(r.now()) != nil {
		return nil
	}

	// Enter the writer to trim an overfull node or to claim a copy that fits.
	var after string
	for {
		keys, more, err := ws.ListObjectsWithType(ctx, s4wave_flowgraph.FlowgraphTypeID, after, 64)
		if err != nil {
			return err
		}
		for _, graphKey := range keys {
			snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, graphKey)
			if err != nil {
				return err
			}
			for nodeID, node := range snapshot.GetState().GetNodes() {
				if !waitsForPlacement(node) || !policy.PermitsNodeType(node.GetTypeId()) {
					continue
				}
				copies, err := r.readRequirementCopies(ctx, ws, graphKey, nodeID, snapshot.GetPlacements()[nodeID])
				if err != nil {
					return err
				}
				requirements := node.GetRequirements()
				overfull := len(copies) > int(requirements.ReplicaCount())
				claim := r.claimable(node, policy, labels, copies) && capacity.Admits(placementRequest(requirements, capacity)) == nil
				if !overfull && !claim {
					continue
				}
				if err := r.claimNode(ctx, reads, graphKey, nodeID, declaration); err != nil {
					return err
				}
			}
			after = graphKey
		}
		if !more {
			return nil
		}
	}
}

// claimNode atomically trims an overfull node, rechecks claimable under the
// writer and reserves on this Device's live Worker claim. A competing commit
// invalidates this transaction; the retry reads the winner's edge and leaves
// the losing Worker without a debit.
func (r *Reconciler) claimNode(ctx context.Context, reads *world.ReadSet, graphKey, nodeID string, declaration *s4wave_device.ForgeWorkerDeclaration) error {
	return world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, tx world.WorldState) error {
		// Re-read the node inside the claim transaction.
		ws := reads.Track(tx)
		snapshot, err := s4wave_flowgraph.ReadFlowgraph(ctx, ws, graphKey)
		if errors.Is(err, world.ErrObjectNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		node := snapshot.GetState().GetNodes()[nodeID]
		if !waitsForPlacement(node) {
			return nil
		}

		// Release only the newest copies, using the ledger's World commit order.
		// Edge and debit disappear together, so a racing Device rechecks the reduced set.
		copies, err := r.readRequirementCopies(ctx, ws, graphKey, nodeID, snapshot.GetPlacements()[nodeID])
		if err != nil {
			return err
		}
		requirements := node.GetRequirements()
		if extra := copies[min(len(copies), int(requirements.ReplicaCount())):]; len(extra) != 0 {
			for _, held := range slices.Backward(extra) {
				if err := releaseRequirementCopy(ctx, ws, graphKey, nodeID, held); err != nil {
					return err
				}
			}
			copies = copies[:len(copies)-len(extra)]
			if err := markPlacementChange(ctx, ws, graphKey); err != nil {
				return err
			}
		}

		// Recheck the claim rule, and that this Device still hosts the declared Worker.
		device, err := world.LookupObjectBody[s4wave_device.Device](ctx, ws, r.deviceKey)
		if err != nil {
			return err
		}
		if !r.claimable(node, r.policy.Snapshot(), device.EffectiveLabels(r.deviceKey), copies) {
			return nil
		}
		placed, err := readPlacedNodes(ctx, ws, r.engineID, r.deviceKey, r.peerID)
		if err != nil || !hostsWorker(placed, declaration.GetWorkerObjectKey()) {
			return err
		}

		// Observe the capacity record that wakes admission when resources return.
		capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, declaration.GetWorkerObjectKey())
		if errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
			return nil
		}
		if err != nil || capacity.OwnerDeviceObjectKey != r.deviceKey {
			return err
		}

		// Reserve under the Worker's current ownership fence a grant whose lifetime is the edge.
		edge, err := s4wave_flowgraph.PlacementQuad(graphKey, nodeID, &s4wave_flowgraph.FlowgraphPlacement{
			Kind:      s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE,
			ObjectKey: r.deviceKey,
		})
		if err != nil {
			return err
		}
		_, err = forge_runtime.NewTxAdmission(r.now).ReserveInTx(
			ctx, ws,
			capacity.WorkerObjectKey,
			s4wave_flowgraph.NodeCapacityHolderKey(graphKey, nodeID, r.deviceKey),
			placementRequest(requirements, capacity),
			&forge_runtime.WorkerClaimRef{DeviceObjectKey: r.deviceKey, ClaimID: capacity.ClaimID},
			capacity.OwnerEpoch,
			world.GraphQuadToQuad(edge),
		)
		if forge_runtime.IsRefusal(err) {
			return nil
		}
		if err != nil {
			return err
		}

		// Replace an expired copy only after our reservation succeeds.
		// Both debits and edges commit together; stale cleanup cannot release our grant.
		if i := slices.IndexFunc(copies, func(held requirementCopy) bool { return held.expired }); i >= 0 {
			if err := releaseRequirementCopy(ctx, ws, graphKey, nodeID, copies[i]); err != nil {
				return err
			}
		}

		// Publish this copy's edge together with its capacity debit.
		if err := ws.SetGraphQuad(ctx, edge); err != nil {
			return err
		}
		return markPlacementChange(ctx, ws, graphKey)
	})
}

// waitsForPlacement reports whether node declares requirements and has no Step
// yet, so Devices may claim copies of it.
func waitsForPlacement(node *s4wave_flowgraph.FlowgraphNode) bool {
	return node.GetRequirements() != nil && node.GetStep() == nil
}

// claimable is the rule both the read scan and the claim transaction apply: the
// node waits for placement, policy permits its type, its selectors match
// labels, and copies has a free slot and none on this Device. Worker capacity
// is the admission's decision.
func (r *Reconciler) claimable(node *s4wave_flowgraph.FlowgraphNode, policy *device_policy.DevicePolicy, labels map[string]string, copies []requirementCopy) bool {
	if !waitsForPlacement(node) || !policy.PermitsNodeType(node.GetTypeId()) {
		return false
	}
	requirements := node.GetRequirements()
	return matchesLabels(requirements, labels) && canClaimCopy(copies, r.deviceKey, requirements.ReplicaCount())
}

// placementRequest is the Worker request for one copy of a node. A node that
// leaves the backend open takes the Worker's first declared backend.
func placementRequest(requirements *s4wave_flowgraph.FlowgraphRequirements, capacity *forge_runtime.WorkerCapacity) forge_runtime.ResourceRequest {
	backend := requirements.GetRuntimeBackend()
	if backend == "" && len(capacity.Backends) != 0 {
		backend = capacity.Backends[0]
	}
	return forge_runtime.ResourceRequest{MilliCPU: requirements.GetMilliCpu(), MemoryBytes: requirements.GetMemoryBytes(), Backend: backend}
}

// hostsWorker reports whether placed holds the forge-worker node for workerKey.
func hostsWorker(placed []*s4wave_flowgraph.PlacedFlowgraphNode, workerKey string) bool {
	return slices.ContainsFunc(placed, func(placedNode *s4wave_flowgraph.PlacedFlowgraphNode) bool {
		return placedNode.Node.GetTypeId() == s4wave_flowgraph.ForgeWorkerNodeTypeID &&
			strings.TrimSpace(placedNode.Node.GetParameters()["worker"]) == workerKey
	})
}

// releaseRemovedPlacements uses the Device's indexed reservation edges to find
// raw edge or object removals. SDK node edits already release in their own write.
func (r *Reconciler) releaseRemovedPlacements(ctx context.Context, reads *world.ReadSet) error {
	// Query only live placement grants belonging to this Device.
	tx, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	ws := reads.Track(tx)
	edges, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", forge_runtime.PlacementReservationPredicate, r.deviceKey, ""), 0)
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}

	// Share one placement read across all of this Device's live grants.
	placed, err := readPlacedNodes(ctx, ws, r.engineID, r.deviceKey, r.peerID)
	if err != nil {
		return err
	}
	for _, edge := range edges {
		key, err := world.GraphValueToKey(edge.GetSubject())
		if err != nil {
			return err
		}
		reservation, err := forge_runtime.LookupReservation(ctx, ws, key)
		if err != nil {
			return err
		}
		live, err := r.placementLive(ctx, ws, reservation, placed)
		if err != nil {
			return err
		}
		if live {
			continue
		}
		if err := r.releasePlacement(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// releasePlacement rechecks a missing edge before crediting an orphaned grant.
func (r *Reconciler) releasePlacement(ctx context.Context, key string) error {
	return world.ExecTransaction(ctx, r.engine, true, func(ctx context.Context, ws world.WorldState) error {
		// A replacement edge or an already released grant makes this cleanup a no-op.
		reservation, err := forge_runtime.LookupReservation(ctx, ws, key)
		if err != nil {
			return err
		}
		if reservation.State.Terminal() {
			return nil
		}

		// A handoff may have reused this holder for another Device since discovery.
		deviceKey, err := world.GraphValueToKey(reservation.Placement.GetObj())
		if err != nil {
			return err
		}
		if deviceKey != r.deviceKey {
			return nil
		}

		// Recheck this Device's placement before deleting an orphaned edge.
		live, err := r.placementLive(ctx, ws, reservation, nil)
		if err != nil || live {
			return err
		}
		if err := ws.DeleteGraphQuad(ctx, reservation.Placement); err != nil {
			return err
		}

		// Commit the credit and terminal receipt in the same cleanup transaction.
		if _, err := forge_runtime.NewTxAdmission(r.now).ReleaseInTx(ctx, ws, key, reservation.ExecutionObjectKey, "", reservation.Generation, true, forge_runtime.CleanupReasonStop); err != nil {
			return err
		}

		// Wake graph Resource watches and close the placement-only history revision.
		graphKey, err := world.GraphValueToKey(reservation.Placement.GetSubject())
		if err != nil {
			return err
		}
		return markPlacementChange(ctx, ws, graphKey)
	})
}

// placementLive checks the edge, node and hosting Worker in one snapshot.
func (r *Reconciler) placementLive(ctx context.Context, ws world.WorldState, reservation *forge_runtime.Reservation, placed []*s4wave_flowgraph.PlacedFlowgraphNode) (bool, error) {
	// A removed graph or placement leaves only the grant to credit.
	edges, err := ws.LookupGraphQuads(ctx, reservation.Placement, 1)
	if err != nil || len(edges) == 0 {
		return false, err
	}

	// Decode the graph and node identity recorded with the grant's edge.
	graphKey, err := world.GraphValueToKey(reservation.Placement.GetSubject())
	if err != nil {
		return false, err
	}
	nodeID, err := strconv.Unquote(reservation.Placement.GetLabel())
	if err != nil {
		return false, err
	}

	// Raw body edits can remove a node while leaving its edge behind.
	graph, err := world.LookupObjectBody[s4wave_flowgraph.Flowgraph](ctx, ws, graphKey)
	if errors.Is(err, world.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil || graph.GetNodes()[nodeID] == nil {
		return false, err
	}

	// A Device without its Worker node returns claimed nodes to unplaced.
	if placed == nil {
		placed, err = readPlacedNodes(ctx, ws, r.engineID, r.deviceKey, r.peerID)
		if err != nil {
			return false, err
		}
	}
	return hostsWorker(placed, reservation.WorkerObjectKey), nil
}

// matchesLabels requires both the presence and the value of each selector key.
func matchesLabels(requirements *s4wave_flowgraph.FlowgraphRequirements, labels map[string]string) bool {
	for key, value := range requirements.GetMatchLabels() {
		if actual, ok := labels[key]; !ok || actual != value {
			return false
		}
	}
	return true
}

// markPlacementChange marks an edge-only edit on its Flowgraph's revision.
// Deleting the whole graph already wakes its watchers and needs no new revision.
func markPlacementChange(ctx context.Context, ws world.WorldState, graphKey string) error {
	// Retain an existing graph while incrementing its transaction-local revision.
	obj, found, err := ws.GetObject(ctx, graphKey)
	defer world.ReleaseObjectState(obj)
	if err != nil || !found {
		return err
	}
	_, err = obj.IncrementRev(ctx)
	return err
}

// requirementCopy holds one edge and its existing Forge grant in a read snapshot.
type requirementCopy struct {
	// placement identifies the copy's Device.
	placement *s4wave_flowgraph.FlowgraphPlacement
	// seqno orders claims by the reservation's last World commit.
	seqno uint64
	// expired records only owner lease expiry, never draining or connectivity.
	expired bool
}

// readRequirementCopies reads each copy's ledger entry and watches its lease.
// Work is proportional to this node's copies, with indexed reads per copy.
func (r *Reconciler) readRequirementCopies(ctx context.Context, ws world.WorldState, graphKey, nodeID string, group *s4wave_flowgraph.FlowgraphPlacements) ([]requirementCopy, error) {
	// Resolve the existing reservation behind each Device edge.
	copies := make([]requirementCopy, 0, len(group.GetCopies()))
	for _, placement := range group.GetCopies() {
		held := requirementCopy{placement: placement}
		key := forge_runtime.BuildReservationObjectKey(s4wave_flowgraph.NodeCapacityHolderKey(graphKey, nodeID, placement.GetObjectKey()))
		reservation, err := forge_runtime.LookupReservation(ctx, ws, key)
		if errors.Is(err, forge_runtime.ErrReservationNotFound) {
			copies = append(copies, held)
			continue
		}
		if err != nil {
			return nil, err
		}

		// The unchanged standing grant retains the World order of its claim.
		obj, found, err := ws.GetObject(ctx, key)
		if err != nil {
			world.ReleaseObjectState(obj)
			return nil, err
		}
		if !found {
			copies = append(copies, held)
			continue
		}
		held.seqno, err = obj.GetModSeqno(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			return nil, err
		}

		// Only the owning Worker lease determines whether this copy is stale.
		capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, reservation.WorkerObjectKey)
		if err != nil {
			return nil, err
		}
		expires := capacity.OwnerLeaseExpiresAt.AsTime()
		held.expired = !r.now().Before(expires)
		if !held.expired && (r.nextExpiry.IsZero() || expires.Before(r.nextExpiry)) {
			r.nextExpiry = expires
		}
		copies = append(copies, held)
	}

	// Resolve equal commit orders by destination key, identically on every Device.
	slices.SortFunc(copies, func(a, b requirementCopy) int {
		return cmp.Or(cmp.Compare(a.seqno, b.seqno), cmp.Compare(a.placement.GetObjectKey(), b.placement.GetObjectKey()))
	})
	return copies, nil
}

// canClaimCopy requires a free slot and no edge already on this Device.
func canClaimCopy(copies []requirementCopy, deviceKey string, count uint32) bool {
	live := 0
	for _, held := range copies {
		if held.placement.GetObjectKey() == deviceKey {
			return false
		}
		if !held.expired {
			live++
		}
	}
	return live < int(count)
}

// releaseRequirementCopy removes an edge and credits its existing Forge grant.
func releaseRequirementCopy(ctx context.Context, ws world.WorldState, graphKey, nodeID string, held requirementCopy) error {
	// Release only the grant named by this Device's edge.
	if err := s4wave_flowgraph.ReleaseNodeCapacity(ctx, ws, graphKey, nodeID, held.placement.GetObjectKey()); err != nil {
		return err
	}
	edge, err := s4wave_flowgraph.PlacementQuad(graphKey, nodeID, held.placement)
	if err != nil {
		return err
	}
	return ws.DeleteGraphQuad(ctx, edge)
}

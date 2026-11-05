package s4wave_flowgraph

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// NodeCapacityHolderKey identifies one standing node copy's grant on its Device.
// Quoted components keep graph and node identities distinct.
func NodeCapacityHolderKey(graphKey, nodeID, deviceKey string) string {
	return "flowgraph-node/" + strconv.Quote(graphKey) + "/" + strconv.Quote(nodeID) + "/" + strconv.Quote(deviceKey)
}

// ReleaseNodeCapacity releases a standing grant in the transaction removing its
// placement. An explicitly placed node has no grant. Execution runtimes cannot
// be released through this path, and a terminal grant is an idempotent success.
func ReleaseNodeCapacity(ctx context.Context, ws world.WorldState, graphKey, nodeID, deviceKey string) error {
	// Look up only this node's deterministic reservation in Forge's shared ledger.
	holder := NodeCapacityHolderKey(graphKey, nodeID, deviceKey)
	reservation, err := forge_runtime.LookupReservation(ctx, ws, forge_runtime.BuildReservationObjectKey(holder))
	if errors.Is(err, forge_runtime.ErrReservationNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if reservation.Placement == nil {
		return nil
	}

	// Credit the same ledger entry as the placement edge disappears.
	_, err = forge_runtime.NewTxAdmission(time.Now).ReleaseInTx(ctx, ws, reservation.ObjectKey(), holder, "", reservation.Generation, true, forge_runtime.CleanupReasonStop)
	return err
}

// ReplicaCount returns the desired copy count, defaulting an unset count to one.
func (r *FlowgraphRequirements) ReplicaCount() uint32 { return max(1, r.GetReplicas()) }

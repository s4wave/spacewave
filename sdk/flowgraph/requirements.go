package s4wave_flowgraph

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// NodeCapacityHolderKey identifies one standing node's grant independently of
// its selected Device. Quoted components keep graph and node identities distinct.
func NodeCapacityHolderKey(graphKey, nodeID string) string {
	return "flowgraph-node/" + strconv.Quote(graphKey) + "/" + strconv.Quote(nodeID)
}

// ReleaseNodeCapacity releases a standing grant in the transaction removing its
// placement. An explicitly placed node has no grant. Execution runtimes cannot
// be released through this path, and a terminal grant is an idempotent success.
func ReleaseNodeCapacity(ctx context.Context, ws world.WorldState, graphKey, nodeID string) error {
	// Look up only this node's deterministic reservation in Forge's shared ledger.
	holder := NodeCapacityHolderKey(graphKey, nodeID)
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
	admission := forge_runtime.NewWorldRuntimeAdmission(nil, nil, 0, 0)
	_, err = admission.ReleaseInTx(ctx, ws, reservation.ObjectKey(), holder, "", reservation.Generation, true, forge_runtime.CleanupReasonStop)
	return err
}

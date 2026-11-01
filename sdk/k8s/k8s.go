// Package s4wave_k8s is the contract between Spacewave and the spacewave-k8s
// plugin, which serves a Space's Kubernetes API: the k8s/cluster object that
// turns the API on.
package s4wave_k8s

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// ClusterObjectKey is the key of a Space's k8s/cluster object.
const ClusterObjectKey = "k8s/cluster"

// EnsureCluster creates the k8s/cluster object in ws unless it exists.
func EnsureCluster(ctx context.Context, ws world.WorldState) error {
	// Leave an existing object.
	found, err := ws.HasObject(ctx, ClusterObjectKey)
	if err != nil || found {
		return err
	}

	// Create it with its type.
	obj, _, err := world.CreateWorldObject(ctx, ws, ClusterObjectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&Cluster{}, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return errors.Wrap(err, "create k8s cluster")
	}
	return world_types.SetObjectType(ctx, ws, ClusterObjectKey, ClusterTypeID)
}

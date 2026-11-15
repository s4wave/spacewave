package forge_worker

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
)

// ListWorkers lists all of the worker keys in the world.
func ListWorkers(ctx context.Context, ws world.WorldState) ([]string, error) {
	var workerKeys []string
	err := world_types.IterateObjectsWithType(ctx, ws, WorkerTypeID, func(objKey string) (bool, error) {
		workerKeys = append(workerKeys, objKey)
		return true, nil
	})
	return workerKeys, err
}

// LookupWorker looks up a worker in the world.
func LookupWorker(ctx context.Context, ws world.WorldState, objKey string) (*Worker, world.ObjectState, error) {
	return world.LookupObject[Worker](ctx, ws, objKey)
}

// CheckWorkerType checks the type graph quad for a worker.
func CheckWorkerType(ctx context.Context, ws world.WorldState, objKey string) error {
	return world_types.CheckObjectType(ctx, ws, objKey, WorkerTypeID)
}

// ListWorkerKeypairs returns the object keys of all Keypairs linked to by the
// given Worker object keys.
func ListWorkerKeypairs(ctx context.Context, w world.WorldState, workerKeys ...string) ([]string, error) {
	return identity_world.ListObjectKeypairs(ctx, w, workerKeys...)
}

// CollectWorkerKeypairs collects all Keypairs linked to by the given Workers,
// with the object key of each.
func CollectWorkerKeypairs(ctx context.Context, w world.WorldState, workerKeys ...string) ([]*identity.Keypair, []string, error) {
	// Find the keypair objects linked to the requested workers.
	kpObjectKeys, err := ListWorkerKeypairs(ctx, w, workerKeys...)
	if err != nil {
		return nil, nil, err
	}

	// Load each linked keypair body in the order of its object key.
	kps := make([]*identity.Keypair, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		var err error
		kps[i], err = identity_world.LookupKeypairBody(ctx, w, objKey)
		if err != nil {
			return nil, kpObjectKeys, err
		}
	}

	return kps, kpObjectKeys, nil
}

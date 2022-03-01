package forge_worker

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/pkg/errors"
)

// LookupWorker looks up a worker in the world.
func LookupWorker(ctx context.Context, ws world.WorldState, objKey string) (*Worker, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, err
	}
	var worker *Worker
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		worker, err = UnmarshalWorker(bcs)
		return err
	})
	return worker, err
}

// CheckWorkerType checks the type graph quad for a worker.
func CheckWorkerType(typesState *world_types.TypesState, objKey string) error {
	workerType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if workerType != WorkerTypeID {
		return errors.Errorf("expected worker type %s but got %q", WorkerTypeID, workerType)
	}
	return err
}

// ListWorkerKeypairs lists all Keypair linked to by the given Worker object keys.
// returns list of object keys
func ListWorkerKeypairs(ctx context.Context, w world.WorldState, workerKeys ...string) ([]string, error) {
	return identity_world.ListObjectKeypairs(ctx, w, workerKeys...)
}

// CollectWorkerKeypairs collects all Keypair linked to by the given entities.
// returns list of Keypair for each object key
func CollectWorkerKeypairs(ctx context.Context, w world.WorldState, workerKeys ...string) ([]*identity.Keypair, []string, error) {
	kpObjectKeys, err := ListWorkerKeypairs(ctx, w, workerKeys...)
	if err != nil {
		return nil, nil, err
	}

	kps := make([]*identity.Keypair, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		kps[i], _, err = identity_world.LookupKeypair(ctx, w, objKey)
		if err != nil {
			return nil, kpObjectKeys, err
		}
	}

	return kps, kpObjectKeys, nil
}

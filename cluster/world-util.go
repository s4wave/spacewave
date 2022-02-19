package forge_cluster

import (
	"context"

	forge_job "github.com/aperturerobotics/forge/job"
	forge_worker "github.com/aperturerobotics/forge/worker"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/cayleygraph/cayley"
	"github.com/pkg/errors"
)

// LookupCluster looks up a cluster in the world.
func LookupCluster(ctx context.Context, ws world.WorldState, objKey string) (*Cluster, error) {
	obj, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return nil, err
	}
	var cluster *Cluster
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var err error
		cluster, err = UnmarshalCluster(bcs)
		return err
	})
	return cluster, err
}

// CheckClusterType checks the type graph quad for a cluster.
func CheckClusterType(typesState *world_types.TypesState, objKey string) error {
	clusterType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if clusterType != ClusterTypeID {
		return errors.Errorf("expected cluster type %s but got %q", ClusterTypeID, clusterType)
	}
	return err
}

// ListClusterJobs lists all Job object keys that are linked to by the Cluster.
func ListClusterJobs(ctx context.Context, w world.WorldState, clusterKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		clusterKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredClusterToJob), nil
		},
	)
}

// CollectClusterJobs collects all active Job linked to by the Cluster.
// If any of the linked states are invalid, returns an error.
func CollectClusterJobs(
	ctx context.Context,
	ws world.WorldState,
	clusterKeys ...string,
) ([]*forge_job.Job, []string, error) {
	kpObjectKeys, err := ListClusterJobs(ctx, ws, clusterKeys...)
	if err != nil {
		return nil, nil, err
	}

	states := make([]*forge_job.Job, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], err = forge_job.LookupJob(ctx, ws, objKey)
		if err == nil {
			err = states[i].Validate()
		}
		if err != nil {
			return nil, nil, errors.Wrapf(err, "jobs[%s]", objKey)
		}
	}

	return states, kpObjectKeys, nil
}

// CheckClusterHasJob checks if the cluster is linked to a job.
func CheckClusterHasJob(ctx context.Context, w world.WorldState, clusterKey, jobKey string) (bool, error) {
	gq, err := w.LookupGraphQuads(world.NewGraphQuad(
		world.KeyToGraphValue(clusterKey).String(),
		PredClusterToJob.String(),
		world.KeyToGraphValue(jobKey).String(),
		"",
	), 1)
	if err != nil {
		return false, err
	}
	return len(gq) != 0, nil
}

// EnsureClusterHasJob checks if the cluster has the job and returns an error otherwise.
func EnsureClusterHasJob(ctx context.Context, w world.WorldState, clusterKey, jobKey string) error {
	hasJob, err := CheckClusterHasJob(ctx, w, clusterKey, jobKey)
	if err == nil && !hasJob {
		err = errors.Errorf("cluster %s does not have job %s", clusterKey, jobKey)
	}
	return err
}

// ListClusterWorkers lists all Worker object keys that are linked to by the Cluster.
func ListClusterWorkers(ctx context.Context, w world.WorldState, clusterKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		clusterKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredClusterToWorker), nil
		},
	)
}

// CollectClusterWorkers collects all Worker linked to by the Cluster.
// If any of the linked states are invalid, returns an error.
func CollectClusterWorkers(
	ctx context.Context,
	ws world.WorldState,
	clusterKeys ...string,
) ([]*forge_worker.Worker, []string, error) {
	kpObjectKeys, err := ListClusterWorkers(ctx, ws, clusterKeys...)
	if err != nil {
		return nil, nil, err
	}

	states := make([]*forge_worker.Worker, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		states[i], err = forge_worker.LookupWorker(ctx, ws, objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return states, kpObjectKeys, nil
}

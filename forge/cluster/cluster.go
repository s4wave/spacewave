// Package forge_cluster defines the Cluster, which schedules Jobs onto the
// Workers linked to it.
//
// A Cluster stores a DNS-label name and the peer that runs its scheduler, and
// links to its Jobs with PredClusterToJob and its Workers with
// PredClusterToWorker. Its typed operations create it, assign its peer, add
// Jobs and Workers, start a Job, assign a Task, and complete a Job. Only the
// assigned peer may submit the scheduling operations. Several Clusters may serve
// one Job, each Execution going to one Worker of one of them, so a Job spreads
// across Clusters without splitting its World state.
package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

const (
	// PredClusterToJob is the predicate linking Cluster to a Job.
	PredClusterToJob = quad.IRI("forge/cluster-job")
	// PredClusterToWorker is the predicate linking Cluster to a Worker.
	PredClusterToWorker = quad.IRI("forge/cluster-worker")
)

// NewClusterBlock constructs a new Cluster block.
func NewClusterBlock() block.Block {
	return &Cluster{}
}

// NewClusterToJobQuad creates a quad linking a Cluster to a Job.
func NewClusterToJobQuad(clusterObjKey, jobObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		clusterObjKey,
		PredClusterToJob.String(),
		jobObjKey,
		"",
	)
}

// NewClusterToWorkerQuad creates a quad linking a Cluster to a Worker.
func NewClusterToWorkerQuad(clusterObjKey, workerObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		clusterObjKey,
		PredClusterToWorker.String(),
		workerObjKey,
		"",
	)
}

// UnmarshalCluster unmarshals a Cluster block from the cursor.
func UnmarshalCluster(ctx context.Context, bcs *block.Cursor) (*Cluster, error) {
	return block.UnmarshalBlock[Cluster](ctx, bcs)
}

// Validate checks the Cluster object cursorily.
func (e *Cluster) Validate() error {
	// Validate the Cluster name and require a nonempty controlling peer.
	if err := identity.ValidateEntityID(e.GetName()); err != nil {
		return err
	}
	pid, err := e.ParsePeerID()
	if err != nil {
		return err
	}
	if pid == "" {
		return peer.ErrEmptyPeerID
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (e *Cluster) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(e.GetPeerId())
}

// _ is a type assertion
var _ block.Block = (*Cluster)(nil)

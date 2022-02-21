package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/identity"
	"github.com/cayleygraph/quad"
	"github.com/golang/protobuf/proto"
)

const (
	// ClusterTypeID is the type identifier for a Cluster.
	ClusterTypeID = "forge/cluster"

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

// LookupClusterOp performs the lookup operation for the Cluster op types.
func LookupClusterOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case ClusterCreateOpId:
		return &ClusterCreateOp{}, nil
	case ClusterAssignJobOpId:
		return &ClusterAssignJobOp{}, nil
	case ClusterAssignWorkerOpId:
		return &ClusterAssignWorkerOp{}, nil
	case ClusterAssignTaskOpId:
		return &ClusterAssignTaskOp{}, nil
	case ClusterAssignPeerOpId:
		return &ClusterAssignPeerOp{}, nil
	}
	return nil, nil
}

// UnmarshalCluster unmarshals a worker block from the cursor.
func UnmarshalCluster(bcs *block.Cursor) (*Cluster, error) {
	vi, err := bcs.Unmarshal(NewClusterBlock)
	if err != nil {
		return nil, err
	}
	if vi == nil {
		return nil, nil
	}
	b, ok := vi.(*Cluster)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return b, nil
}

// Validate performs cursory checks of the Cluster object.
func (e *Cluster) Validate() error {
	if err := identity.ValidateEntityID(e.GetName()); err != nil {
		return err
	}
	pid, err := e.ParsePeerID()
	if err != nil {
		return err
	}
	if pid == "" {
		return peer.ErrPeerIDEmpty
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (e *Cluster) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(e.GetPeerId())
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Cluster) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Cluster) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// _ is a type assertion
var _ block.Block = ((*Cluster)(nil))

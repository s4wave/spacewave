package forge_dashboard

import (
	"context"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_web_object "github.com/s4wave/spacewave/web/object"
	"github.com/sirupsen/logrus"
)

// InitForgeQuickstartOpId is the operation id for InitForgeQuickstartOp.
var InitForgeQuickstartOpId = "spacewave/forge/quickstart/init"

// GetOperationTypeId returns the operation type identifier.
func (o *InitForgeQuickstartOp) GetOperationTypeId() string {
	return InitForgeQuickstartOpId
}

// Validate performs cursory checks on the op.
func (o *InitForgeQuickstartOp) Validate() error {
	if len(o.GetLayoutKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetDashboardKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetClusterKey()) == 0 {
		return world.ErrEmptyObjectKey
	}
	if len(o.GetSessionPeerId()) == 0 {
		return peer.ErrEmptyPeerID
	}
	if err := o.GetTimestamp().Validate(false); err != nil {
		return err
	}
	return s4wave_layout_world.CheckObjectLayoutKey(o.GetLayoutKey())
}

// ApplyWorldOp applies the operation as a world operation.
func (o *InitForgeQuickstartOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}

	// Parse the session peer ID for entity ownership.
	sessionPeerID, err := confparse.ParsePeerID(o.GetSessionPeerId())
	if err != nil {
		return false, err
	}

	dashKey := o.GetDashboardKey()
	clusterKey := o.GetClusterKey()

	// Create the dashboard.
	dashboard := &ForgeDashboard{
		Name:      "Forge Dashboard",
		CreatedAt: o.GetTimestamp(),
	}
	_, _, err = world.CreateWorldObject(ctx, ws, dashKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(dashboard, true)
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := world_types.SetObjectType(ctx, ws, dashKey, ForgeDashboardTypeID); err != nil {
		return false, err
	}

	// Create the cluster with the session peer ID as the cluster identity.
	clusterCreateOp := forge_cluster.NewClusterCreateOp(clusterKey, o.GetClusterName(), sessionPeerID)
	if _, _, err := ws.ApplyWorldOp(ctx, clusterCreateOp, sessionPeerID); err != nil {
		return false, err
	}

	// Create a sample job with tasks under the cluster.
	jobKey := clusterKey + "/job/sample"
	tasks := map[string]*forge_target.Target{
		"compile": space_exec.NewNoopTarget(),
		"link":    space_exec.NewNoopTarget(),
		"test":    space_exec.NewNoopTarget(),
	}
	_, _, err = forge_job.CreateJobWithTasks(ctx, ws, sessionPeerID, jobKey, tasks, "", o.GetTimestamp())
	if err != nil {
		return false, err
	}

	// Assign the job to the cluster.
	assignJobOp := forge_cluster.NewClusterAssignJobOp(clusterKey, jobKey)
	if _, _, err := ws.ApplyWorldOp(ctx, assignJobOp, sessionPeerID); err != nil {
		return false, err
	}

	// Link cluster and job to dashboard.
	linkClusterOp := NewLinkForgeDashboardOp(dashKey, clusterKey)
	if _, _, err := ws.ApplyWorldOp(ctx, linkClusterOp, sessionPeerID); err != nil {
		return false, err
	}
	linkJobOp := NewLinkForgeDashboardOp(dashKey, jobKey)
	if _, _, err := ws.ApplyWorldOp(ctx, linkJobOp, sessionPeerID); err != nil {
		return false, err
	}

	// Create the worker if requested.
	workerKey := o.GetWorkerKey()
	if workerKey != "" {
		workerCreateOp := forge_worker.NewWorkerCreateOp(workerKey, "session-worker", nil)
		if _, _, err := ws.ApplyWorldOp(ctx, workerCreateOp, sessionPeerID); err != nil {
			return false, err
		}

		// Link the worker to the session's keypair.
		kpKey := identity_world.NewKeypairKey(o.GetSessionPeerId())
		if err := ws.SetGraphQuad(ctx, identity_world.NewObjectToKeypairQuad(workerKey, kpKey)); err != nil {
			return false, err
		}

		// Assign worker to the cluster.
		assignWorkerOp := forge_cluster.NewClusterAssignWorkerOp(clusterKey, workerKey)
		if _, _, err := ws.ApplyWorldOp(ctx, assignWorkerOp, sessionPeerID); err != nil {
			return false, err
		}

		// Link worker to dashboard.
		linkWorkerOp := NewLinkForgeDashboardOp(dashKey, workerKey)
		if _, _, err := ws.ApplyWorldOp(ctx, linkWorkerOp, sessionPeerID); err != nil {
			return false, err
		}
	}

	// Create the ObjectLayout with a dashboard tab.
	layoutKey := o.GetLayoutKey()
	layout := &s4wave_layout_world.ObjectLayout{
		LayoutModel: &s4wave_layout.LayoutModel{
			Layout: &s4wave_layout.RowDef{
				Id: "root",
				Children: []*s4wave_layout.RowOrTabSetDef{
					{
						Node: &s4wave_layout.RowOrTabSetDef_TabSet{
							TabSet: &s4wave_layout.TabSetDef{
								Id:     "main-tabset",
								Weight: 100,
								Children: []*s4wave_layout.TabDef{
									{
										Id:   "dashboard",
										Name: "Dashboard",
										Data: s4wave_layout_world.NewObjectLayoutTab(
											"",
											&s4wave_web_object.ObjectInfo{
												Info: &s4wave_web_object.ObjectInfo_WorldObjectInfo{
													WorldObjectInfo: &s4wave_web_object.WorldObjectInfo{
														ObjectKey: dashKey,
													},
												},
											},
											"",
										).Marshal(),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, _, err = world.CreateWorldObject(ctx, ws, layoutKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(layout, true)
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := world_types.SetObjectType(ctx, ws, layoutKey, s4wave_layout_world.ObjectLayoutTypeID); err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *InitForgeQuickstartOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *InitForgeQuickstartOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *InitForgeQuickstartOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupInitForgeQuickstartOp looks up an InitForgeQuickstartOp operation type.
func LookupInitForgeQuickstartOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == InitForgeQuickstartOpId {
		return &InitForgeQuickstartOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*InitForgeQuickstartOp)(nil))

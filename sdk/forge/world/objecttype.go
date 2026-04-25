// Package s4wave_forge_world registers Forge ObjectTypes in the Space World.
package s4wave_forge_world

import (
	"context"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	"github.com/sirupsen/logrus"
)

// ClusterType is the ObjectType for forge/cluster objects.
var ClusterType = objecttype.NewObjectType(forge_cluster.ClusterTypeID, ForgeReadOnlyFactory)

// JobType is the ObjectType for forge/job objects.
var JobType = objecttype.NewObjectType(forge_job.JobTypeID, ForgeReadOnlyFactory)

// TaskType is the ObjectType for forge/task objects.
var TaskType = objecttype.NewObjectType(forge_task.TaskTypeID, ForgeReadOnlyFactory)

// PassType is the ObjectType for forge/pass objects.
var PassType = objecttype.NewObjectType(forge_pass.PassTypeID, ForgeReadOnlyFactory)

// ExecutionType is the ObjectType for forge/execution objects.
var ExecutionType = objecttype.NewObjectType(forge_execution.ExecutionTypeID, ForgeReadOnlyFactory)

// WorkerType is the ObjectType for forge/worker objects.
// Uses forgeWorkerFactory which returns a PersistentExecutionService invoker
// when the session peer ID matches the Worker's linked keypair.
var WorkerType = objecttype.NewObjectType(forge_worker.WorkerTypeID, forgeWorkerFactory)

// DashboardType is the ObjectType for spacewave/forge/dashboard objects.
var DashboardType = objecttype.NewObjectType(forge_dashboard.ForgeDashboardTypeID, ForgeReadOnlyFactory)

// ForgeReadOnlyFactory is a minimal factory for read-only object types.
// Viewers access block state through the objectState prop directly.
func ForgeReadOnlyFactory(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	ws world.WorldState,
	objectKey string,
) (srpc.Invoker, func(), error) {
	if ws == nil {
		return nil, nil, objecttype.ErrWorldStateRequired
	}
	return nil, func() {}, nil
}

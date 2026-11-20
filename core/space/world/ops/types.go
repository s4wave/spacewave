package space_world_ops

import (
	"slices"

	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	forge_world "github.com/s4wave/spacewave/forge/world"
	s4wave_camera "github.com/s4wave/spacewave/sdk/camera"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// Types lists the built-in Space World operations.
//
// buildTypes adds the operations whose packages differ between builds.
var Types = slices.Concat(
	OpsTypes,
	unixfs_world.UnixfsTypes,
	spacewave_chat.ChatTypes,
	s4wave_device.DeviceTypes,
	s4wave_flowgraph.FlowgraphTypes,
	s4wave_org.OrgTypes,
	s4wave_terminal.TerminalTypes,
	s4wave_camera.CameraTypes,
	s4wave_vm.V86Types,
	forge_world.Types,
	forge_dashboard.DashboardTypes,
	forge_job_ops.JobTypes,
	forge_task_ops.TaskTypes,
	buildTypes,
)

//go:build !tinygo

package objecttypes

import (
	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	s4wave_canvas_world "github.com/s4wave/spacewave/sdk/canvas/world"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_world "github.com/s4wave/spacewave/sdk/chat/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_device_world "github.com/s4wave/spacewave/sdk/device/world"
	s4wave_forge_world "github.com/s4wave/spacewave/sdk/forge/world"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_sshhost_world "github.com/s4wave/spacewave/sdk/sshhost/world"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_terminal_world "github.com/s4wave/spacewave/sdk/terminal/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

var commonObjectTypes = map[string]objecttype.ObjectType{
	s4wave_layout_world.ObjectLayoutTypeID: s4wave_layout_world.ObjectLayoutType,
	s4wave_unixfs_world.UnixFSTypeID:       s4wave_unixfs_world.UnixFSType,
	s4wave_git_world.GitRepoTypeID:         s4wave_git_world.GitRepoType,
	s4wave_canvas_world.CanvasTypeID:       s4wave_canvas_world.CanvasType,
	s4wave_git_world.GitWorktreeTypeID:     s4wave_git_world.GitWorktreeType,
	s4wave_kv_world.KvStoreTypeID:          s4wave_kv_world.KvStoreType,
	forge_cluster.ClusterTypeID:            s4wave_forge_world.ClusterType,
	forge_job.JobTypeID:                    s4wave_forge_world.JobType,
	forge_task.TaskTypeID:                  s4wave_forge_world.TaskType,
	forge_pass.PassTypeID:                  s4wave_forge_world.PassType,
	forge_execution.ExecutionTypeID:        s4wave_forge_world.ExecutionType,
	forge_worker.WorkerTypeID:              s4wave_forge_world.WorkerType,
	forge_dashboard.ForgeDashboardTypeID:   s4wave_forge_world.DashboardType,
	spacewave_chat.ChatChannelTypeID:       spacewave_chat_world.ChatChannelType,
	spacewave_chat.ChatMessageTypeID:       spacewave_chat_world.ChatMessageType,
	s4wave_device.DeviceTypeID:             s4wave_device_world.DeviceType,
	s4wave_device.ComputersDashboardTypeID: s4wave_device_world.ComputersDashboardType,
	s4wave_terminal.TerminalTypeID:         s4wave_terminal_world.TerminalType,
	s4wave_sshhost.SshHostTypeID:           s4wave_sshhost_world.SshHostType,
}

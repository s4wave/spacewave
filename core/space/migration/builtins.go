package space_migration

import (
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
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
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_org_world "github.com/s4wave/spacewave/sdk/org/world"
	s4wave_secret_world "github.com/s4wave/spacewave/sdk/secret/world"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_sshhost_world "github.com/s4wave/spacewave/sdk/sshhost/world"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_terminal_world "github.com/s4wave/spacewave/sdk/terminal/world"
	s4wave_unixfs_world "github.com/s4wave/spacewave/sdk/unixfs/world"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_vm_world "github.com/s4wave/spacewave/sdk/vm/world"
)

const spaceSettingsTypeID = "github.com/s4wave/spacewave/core/space/world.SpaceSettings"

// BuiltInRegistry returns the complete built-in ObjectType classification.
func BuiltInRegistry() (*Registry, error) {
	registry := NewRegistry()
	handlers := []*TypedHandler{
		NewSchemaHandler(spaceSettingsTypeID, ClassificationRewrite, true, false, false, false, false, inspectSpaceSettings, rewriteSpaceSettings),
		NewSchemaHandler(s4wave_layout_world.ObjectLayoutTypeID, ClassificationRewrite, true, false, false, false, false, inspectObjectLayout, rewriteObjectLayout),
		NewSchemaHandler(s4wave_unixfs_world.UnixFSTypeID, ClassificationRewrite, false, false, false, false, false, inspectUnixFS, rewriteUnixFS),
		NewSchemaHandler(s4wave_git_world.GitRepoTypeID, ClassificationRewrite, false, false, false, true, false, inspectGitRepo, rewriteGitRepo),
		NewSchemaHandler(s4wave_git_world.GitWorktreeTypeID, ClassificationRewrite, true, false, false, true, false, inspectGitWorktree, rewriteGitWorktree),
		NewSchemaHandler(s4wave_canvas_world.CanvasTypeID, ClassificationRewrite, true, false, true, true, false, inspectCanvas, rewriteCanvas),
		NewSchemaHandler(s4wave_kv_world.KvStoreTypeID, ClassificationSpaceLocalOpaque, false, false, false, false, false, inspectKV, rewriteKV),
		NewSchemaHandler(forge_cluster.ClusterTypeID, ClassificationRewrite, false, false, false, true, false, inspectForgeCluster, rewriteForgeCluster),
		NewSchemaHandler(forge_job.JobTypeID, ClassificationRewrite, false, false, false, true, false, inspectForgeJob, rewriteForgeJob),
		NewSchemaHandler(forge_task.TaskTypeID, ClassificationRewrite, true, false, false, true, true, inspectForgeTask, rewriteForgeTask),
		NewSchemaHandler(forge_pass.PassTypeID, ClassificationRewrite, true, false, false, true, true, inspectForgePass, rewriteForgePass),
		NewSchemaHandler(forge_execution.ExecutionTypeID, ClassificationRewrite, true, false, false, true, true, inspectForgeExecution, rewriteForgeExecution),
		NewSchemaHandler(forge_worker.WorkerTypeID, ClassificationRewrite, false, false, false, true, false, inspectForgeWorker, rewriteForgeWorker),
		NewSchemaHandler(forge_dashboard.ForgeDashboardTypeID, ClassificationRewrite, false, false, false, true, false, inspectForgeDashboard, rewriteForgeDashboard),
		NewSchemaHandler(spacewave_chat.ChatChannelTypeID, ClassificationRewrite, false, false, false, true, false, inspectChatChannel, rewriteChatChannel),
		NewSchemaHandler(spacewave_chat.ChatMessageTypeID, ClassificationRewrite, true, false, false, false, false, inspectChatMessage, rewriteChatMessage),
		NewSchemaHandler(s4wave_device.DeviceTypeID, ClassificationRewrite, true, false, false, false, false, inspectDevice, rewriteDevice),
		NewSchemaRefusalHandler(s4wave_device.ComputersDashboardTypeID, ClassificationExternalRef, "device dashboard payload is external and not admitted for rewrite"),
		NewSchemaHandler(s4wave_terminal.TerminalTypeID, ClassificationRewrite, true, false, false, false, false, inspectTerminal, rewriteTerminal),
		NewSchemaHandler(s4wave_sshhost.SshHostTypeID, ClassificationRewrite, true, false, false, false, false, inspectSSHHost, rewriteSSHHost),
		NewSchemaRefusalHandler(s4wave_vm.VmV86TypeID, ClassificationNonMigratable, "V86 runtime state is non-migratable"),
		NewSchemaRefusalHandler(s4wave_vm.V86ImageTypeID, ClassificationExternalRef, "V86 image payload is external and not admitted for rewrite"),
		NewSchemaRefusalHandler(s4wave_org.OrganizationTypeID, ClassificationNonMigratable, "organization state is non-migratable"),
		NewSchemaHandler(s4wave_secret_world.SecretTypeID, ClassificationRewrite, false, true, false, false, true, inspectSecret, rewriteSecret),
		NewSchemaRefusalHandler(bldr_manifest_world.ManifestTypeID, ClassificationExternalRef, "manifest payload is external and not admitted for rewrite"),
	}
	for _, handler := range handlers {
		if err := registry.Register(handler); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Keep imports for built-in world ObjectType packages visible to the registry.
var _ = s4wave_org_world.OrganizationType
var _ = s4wave_device_world.DeviceType
var _ = s4wave_terminal_world.TerminalType
var _ = s4wave_sshhost_world.SshHostType
var _ = s4wave_vm_world.VmV86Type
var _ = spacewave_chat_world.ChatChannelType

//go:build !tinygo

package space_world

import (
	"context"
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	"github.com/s4wave/spacewave/bldr/manifest/builder/resultworld"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	forge_dashboard "github.com/s4wave/spacewave/core/forge/dashboard"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/blocktype"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	s4wave_apt "github.com/s4wave/spacewave/sdk/apt"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_chat "github.com/s4wave/spacewave/sdk/chat"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// applicationBlockTypes supplies the same persisted roots used by application
// resources. Plugin-specific roots continue through their registered controller.
var applicationBlockTypes = map[string]blocktype.BlockType{
	identity_world.KeypairTypeID:             blocktype.NewBlockType(identity_world.KeypairTypeID, identity.NewKeypairBlock),
	bldr_manifest_world.ManifestTypeID:       blocktype.NewBlockType(bldr_manifest_world.ManifestTypeID, bldr_manifest.NewManifestBlock),
	bldr_manifest_world.ManifestBundleTypeID: blocktype.NewBlockType(bldr_manifest_world.ManifestBundleTypeID, bldr_manifest.NewManifestBundleBlock),
	resultworld.ManifestBuildResultTypeID:    blocktype.NewBlockType(resultworld.ManifestBuildResultTypeID, bldr_manifest_builder.NewBuilderResultBlock),
	forge_dashboard.ForgeDashboardTypeID:     blocktype.NewBlockType(forge_dashboard.ForgeDashboardTypeID, func() *forge_dashboard.ForgeDashboard { return &forge_dashboard.ForgeDashboard{} }),
	forge_cluster.ClusterTypeID:              blocktype.NewBlockType(forge_cluster.ClusterTypeID, forge_cluster.NewClusterBlock),
	forge_execution.ExecutionTypeID:          blocktype.NewBlockType(forge_execution.ExecutionTypeID, forge_execution.NewExecutionBlock),
	forge_job.JobTypeID:                      blocktype.NewBlockType(forge_job.JobTypeID, forge_job.NewJobBlock),
	forge_pass.PassTypeID:                    blocktype.NewBlockType(forge_pass.PassTypeID, forge_pass.NewPassBlock),
	forge_task.TaskTypeID:                    blocktype.NewBlockType(forge_task.TaskTypeID, forge_task.NewTaskBlock),
	forge_worker.WorkerTypeID:                blocktype.NewBlockType(forge_worker.WorkerTypeID, forge_worker.NewWorkerBlock),
	s4wave_apt.AptRepositoryTypeID:           blocktype.NewBlockType(s4wave_apt.AptRepositoryTypeID, s4wave_apt.NewAptRepositoryBlock),
	s4wave_apt.AptPackageTypeID:              blocktype.NewBlockType(s4wave_apt.AptPackageTypeID, s4wave_apt.NewAptPackageBlock),
	s4wave_apt.AptBuildSpecTypeID:            blocktype.NewBlockType(s4wave_apt.AptBuildSpecTypeID, s4wave_apt.NewAptBuildSpecBlock),
	"canvas":                                 blocktype.NewBlockType("canvas", s4wave_canvas.NewCanvasStorageBlock),
	"kv/store":                               blocktype.NewBlockType("kv/store", kvtx_block.NewKeyValueStoreBlock),
	git_world.GitRepoTypeID:                  blocktype.NewBlockType(git_world.GitRepoTypeID, git_block.NewRepoBlock),
	git_world.GitWorktreeTypeID:              blocktype.NewBlockType(git_world.GitWorktreeTypeID, git_world.NewWorktreeBlock),
	s4wave_chat.ChatChannelTypeID:            blocktype.NewBlockType(s4wave_chat.ChatChannelTypeID, s4wave_chat.NewChatChannelBlock),
	s4wave_chat.ChatMessageTypeID:            blocktype.NewBlockType(s4wave_chat.ChatMessageTypeID, s4wave_chat.NewChatMessageBlock),
	s4wave_device.DeviceTypeID:               blocktype.NewBlockType(s4wave_device.DeviceTypeID, s4wave_device.NewDeviceBlock),
	s4wave_device.ComputersDashboardTypeID:   blocktype.NewBlockType(s4wave_device.ComputersDashboardTypeID, func() *s4wave_device.ComputersDashboard { return &s4wave_device.ComputersDashboard{} }),
	s4wave_org.OrganizationTypeID:            blocktype.NewBlockType(s4wave_org.OrganizationTypeID, s4wave_org.NewOrgStateBlock),
	s4wave_secret.SecretTypeID:               blocktype.NewBlockType(s4wave_secret.SecretTypeID, s4wave_secret.NewSecretBlock),
	s4wave_sshhost.SshHostTypeID:             blocktype.NewBlockType(s4wave_sshhost.SshHostTypeID, s4wave_sshhost.NewSshHostBlock),
	s4wave_terminal.TerminalTypeID:           blocktype.NewBlockType(s4wave_terminal.TerminalTypeID, s4wave_terminal.NewTerminalBlock),
	s4wave_vm.VmV86TypeID:                    blocktype.NewBlockType(s4wave_vm.VmV86TypeID, func() *s4wave_vm.VmV86 { return &s4wave_vm.VmV86{} }),
}

// lookupApplicationBlockType resolves application object roots for block traversal.
func lookupApplicationBlockType(_ context.Context, typeID string) (blocktype.BlockType, error) {
	// SDK app bindings are JSON leaves. Collections and immutable executables
	// have their own typed World objects, so decoding never needs plugin code.
	if strings.HasPrefix(typeID, "sync/app/") {
		return blocktype.NewBlockType(typeID, byteslice.NewByteSliceBlock), nil
	}
	if strings.HasPrefix(typeID, "wizard/") {
		return blocktype.NewBlockType(typeID, s4wave_wizard.NewWizardStateBlock), nil
	}
	return applicationBlockTypes[typeID], nil
}

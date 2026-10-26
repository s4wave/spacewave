package optypes

import (
	"context"
	"strings"

	"github.com/aperturerobotics/common/typeid"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/blocktype"
	git_block "github.com/s4wave/spacewave/db/git/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/identity"
	s4wave_alarm "github.com/s4wave/spacewave/sdk/alarm"
	s4wave_appconnector "github.com/s4wave/spacewave/sdk/appconnector"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
	s4wave_sshhost "github.com/s4wave/spacewave/sdk/sshhost"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// schema resolves the Space world operation and block types.
//
// buildTypes adds the types whose packages differ between builds. Plugins
// resolve the operations and block types they own, such as SQL, themselves.
var schema = world.NewSchema(
	space_world_ops.Types,
	s4wave_kv_world.WorldTypes,
	s4wave_alarm.AlarmTypes,
	s4wave_appconnector.AppconnectorTypes,
	s4wave_sshhost.SshhostTypes,
	s4wave_wizard.WizardTypes,
	space_world.WorldTypes,
	s4wave_layout_world.WorldTypes,
	unixfs_block.FstreeTypes,
	forge_execution.ExecutionTypes,
	forge_job.JobTypes,
	forge_pass.PassTypes,
	forge_task.TaskTypes,
	identity.IdentityTypes,
	bldr_manifest.ManifestTypes,
	s4wave_canvas.StorageTypes,
	kvtx_block.KvtxTypes,
	git_block.GitTypes,
	[]typeid.Type{
		// The Python resource package publishes the Secret message, and no
		// Python package ships typeid.proto, so Secret declares its ID here.
		{ID: s4wave_secret.SecretTypeID, New: func() any { return new(s4wave_secret.Secret) }},
		// VM images resolve here because the CDN catalog Space that publishes
		// them is mounted without the WasiVM plugin.
		{ID: s4wave_vm.WasiVmImageTypeID, New: func() any { return new(s4wave_vm.WasiVmImage) }},
		// Older Spaces stored their settings under the Go type name.
		{ID: "github.com/s4wave/spacewave/core/space/world.SpaceSettings", New: func() any { return new(space_world.SpaceSettings) }},
	},
	buildTypes,
)

// LookupWorldOp looks up the available world operation types.
func LookupWorldOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	return schema.LookupOp(ctx, opTypeID)
}

// LookupBlockType looks up the Space block types.
func LookupBlockType(ctx context.Context, typeID string) (blocktype.BlockType, error) {
	// SDK app bindings are JSON leaves and wizards share one state block, so
	// both resolve by prefix.
	switch {
	case strings.HasPrefix(typeID, "sync/app/"):
		return blocktype.New[byteslice.ByteSlice](typeID), nil
	case strings.HasPrefix(typeID, "wizard/"):
		return blocktype.New[s4wave_wizard.WizardState](typeID), nil
	}
	return schema.LookupBlockType(ctx, typeID)
}

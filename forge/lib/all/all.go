package forge_lib

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"

	forge_git "github.com/s4wave/spacewave/forge/lib/git"
	forge_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_skiffos "github.com/s4wave/spacewave/forge/lib/skiffos"
	forge_skiffos_audit "github.com/s4wave/spacewave/forge/lib/skiffos/audit"
	forge_skiffos_update "github.com/s4wave/spacewave/forge/lib/skiffos/update"
	forge_util "github.com/s4wave/spacewave/forge/lib/util"
	forge_v86 "github.com/s4wave/spacewave/forge/lib/v86"
	os_image_sign "github.com/s4wave/spacewave/os/image/sign"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// Register every library controller with the resolver.
	forge_kvtx.AddFactories(b, sr)
	forge_git.AddFactories(b, sr)
	forge_util.AddFactories(b, sr)
	forge_v86.AddFactories(b, sr)
	forge_skiffos.AddFactories(sr)
	forge_skiffos_audit.AddFactories(sr)
	forge_skiffos_update.AddFactories(sr)
	os_image_sign.AddFactories(b, sr)
}

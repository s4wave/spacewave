package spacewave_launcher_controller

// Blank imports registering the controller packages that the
// spacewave-launcher bldr manifest lists under goPkgs. Each one must be
// reachable via a Go import so bldr's manifest builder can resolve the
// package and compile it into the plugin bus.
import (
	_ "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	_ "github.com/s4wave/spacewave/core/cdn/world/controller"
	_ "github.com/s4wave/spacewave/core/space/world/optypes"
	_ "github.com/s4wave/spacewave/db/block/store/overlay"
	_ "github.com/s4wave/spacewave/db/block/store/rpc/server"
	_ "github.com/s4wave/spacewave/db/object/peer"
)

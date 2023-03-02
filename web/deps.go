//go:build deps_only
// +build deps_only

package bldr_web

// Import all Go packages which are referenced by web/ .proto files.
import (
	// _ imports BlockRef
	_ "github.com/aperturerobotics/hydra/block"
	// _ imports ControllerConfig from configset
	_ "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	// _ imports RpcStreamPacket
	_ "github.com/aperturerobotics/starpc/rpcstream"
	// _ imports ExecControllerRequest
	_ "github.com/aperturerobotics/controllerbus/controller/exec"
	// _ imports ObjectRef
	_ "github.com/aperturerobotics/hydra/bucket"
	// _ imports bldr_values
	_ "github.com/aperturerobotics/bldr/values"
	// _ imports the electron web plugin entrypoint
	_ "github.com/aperturerobotics/bldr/web/plugin/electron/entrypoint"
)

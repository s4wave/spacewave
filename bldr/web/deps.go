//go:build deps_only

package bldr_web

// Import all Go packages which are referenced by web/ .proto files.
import (
	// _ imports BlockRef
	_ "github.com/s4wave/spacewave/db/block"
	// _ imports ControllerConfig from configset
	_ "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	// _ imports RpcStreamPacket
	_ "github.com/aperturerobotics/starpc/rpcstream"
	// _ imports ExecControllerRequest
	_ "github.com/aperturerobotics/controllerbus/controller/exec"
	// _ imports ObjectRef
	_ "github.com/s4wave/spacewave/db/bucket"
	// _ imports bldr_values
	_ "github.com/s4wave/spacewave/bldr/values"
	// _ imports cpp-yamux for saucer C++ binding
	_ "github.com/aperturerobotics/cpp-yamux"
)

package devtool_web

import "github.com/aperturerobotics/bifrost/protocol"

// HostServiceIDPrefix is the prefix used for the devtool RPC services. This
// ID can be prepended to RPC service IDs to indicate the service is located on
// the devtool (while running within the web runtime).
const HostServiceIDPrefix = "devtool/"

// HostServerID is the server ID used for devtool-host originating RPC calls.
const HostServerID = "devtool/web"

// HostProtocolID is the protocol ID used for devtool-host RPC calls.
const HostProtocolID = protocol.ID("devtool/web/rpc")

// EntrypointClientID is the client ID used for devtool-entrypoint originating RPC calls.
const EntrypointClientID = "devtool/web/entrypoint"

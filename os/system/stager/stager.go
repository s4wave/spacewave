// Package os_system_stager is the protocol between a placed os/system node
// and the console of its Device. The node serves the Stager and the Space's
// blocks on the Device's bus; the daemon's socket routes every service under
// ServicePrefix to it; and the console's stage child reads both and writes the
// image store. The package stays apart from the node type so that init, which
// runs the stage child, links only the protocol.
package os_system_stager

// ServicePrefix prefixes the ID of every service a placed node serves on its
// Device's daemon socket.
const ServicePrefix = "os.system."

// BlocksServiceID is the ID under which a placed node serves the Space's
// blocks, read only, with the block.rpc BlockStore service.
const BlocksServiceID = ServicePrefix + "Blocks"

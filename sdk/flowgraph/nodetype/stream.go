package flowgraph_nodetype

import (
	"net/netip"
	"strconv"

	"github.com/pkg/errors"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

const (
	// streamPortName is the name of the one port of a TCP Port or Local Port.
	streamPortName = "stream"
	// streamPortTypeID is the data contract of a TCP stream.
	streamPortTypeID = "stream"
	// addressParameter names the node parameter holding a TCP address.
	addressParameter = "address"
)

// streamProtocolID derives the peer stream protocol ID of a connection from
// its Flowgraph and connection IDs, so connections never share a protocol.
func streamProtocolID(flowgraphKey, connectionID string) string {
	return "spacewave/" + flowgraphKey + "/" + connectionID
}

// streamMultiaddr converts the node's address parameter, an IP and a port, to a
// TCP multiaddress.
func streamMultiaddr(node *s4wave_flowgraph.FlowgraphNode) (string, error) {
	// Parse the address as an IP and a port.
	address := node.GetParameters()[addressParameter]
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return "", errors.Wrapf(err, "parameter %q must be an IP address and a port", addressParameter)
	}

	// Select the multiaddress network from the IP version.
	network := "ip4"
	if addrPort.Addr().Is6() {
		network = "ip6"
	}
	return "/" + network + "/" + addrPort.Addr().String() + "/tcp/" + strconv.Itoa(int(addrPort.Port())), nil
}

// requireDevicePeerID rejects a node placed on no Device.
func requireDevicePeerID(node *s4wave_flowgraph.PlacedFlowgraphNode) error {
	if node.DevicePeerID == "" {
		return errors.New("node is not placed on a Device")
	}
	return nil
}

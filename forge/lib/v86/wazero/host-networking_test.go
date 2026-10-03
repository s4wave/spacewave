package v86_wazero

import (
	"context"
	"encoding/binary"
	"testing"
)

// newNetworkingTestHost builds a HostRuntime with a PCI device and networking
// wired, mirroring the production registerNetworking path without a wasm CPU.
func newNetworkingTestHost(t *testing.T) *HostRuntime {
	// Create the HostRuntime with I/O tracking and PCI register state.
	t.Helper()
	host := &HostRuntime{
		ioPorts:      newIOPorts(),
		ioReads:      make(map[uint16]uint64),
		ioWrites:     make(map[uint16]uint64),
		ioLastReads:  make(map[uint16]uint32),
		ioLastWrites: make(map[uint16]uint32),
	}
	host.pci = &pciDevice{
		host:      host,
		spaces:    make(map[uint16][]byte),
		barSizes:  make(map[uint16]map[int]uint32),
		barIO:     make(map[uint16]map[int]bool),
		barProbes: make(map[uint16]map[int]bool),
	}

	// Register the host network stack and release it when the test ends.
	host.registerNetworking(context.Background(), NetworkConfig{})
	t.Cleanup(func() { _ = host.network.Close() })

	return host
}

// TestRegisterNetworkingARPRoundTrip proves the wiring glue: a guest ARP request
// for the gateway flows through SetOutbound into the stack, the stack's reply
// reaches the device through the inbound sink (QueueInbound), the wake channel is
// signaled, and drainNetwork delivers the reply into the receive ring.
func TestRegisterNetworkingARPRoundTrip(t *testing.T) {
	// Verify host networking registers the NE2K device and wake channel.
	host := newNetworkingTestHost(t)
	if host.ne2k == nil || host.network == nil || host.netWake == nil {
		t.Fatal("registerNetworking left a nil device, stack, or wake channel")
	}

	// Send the guest gateway ARP request through the network stack.
	frame := buildGuestARPRequest(defaultGuestMAC, [4]byte{10, 0, 2, 15}, [4]byte{10, 0, 2, 2})
	host.ne2k.outbound(frame)

	// Verify the inbound ARP reply signals the host wake channel.
	select {
	case <-host.netWakeCh():
	default:
		t.Fatal("inbound sink did not signal the wake channel")
	}

	// Verify the ARP reply waits in the NE2K inbound queue.
	host.ne2k.mtx.Lock()
	queued := len(host.ne2k.inbound)
	host.ne2k.mtx.Unlock()
	if queued != 1 {
		t.Fatalf("queued inbound frames = %d, want 1 (the ARP reply)", queued)
	}

	// drainNetwork hands the queue to the device; ring delivery itself is
	// covered by the NE2K device tests.
	host.drainNetwork(context.Background())
	host.ne2k.mtx.Lock()
	remaining := len(host.ne2k.inbound)
	host.ne2k.mtx.Unlock()
	if remaining != 0 {
		t.Fatalf("inbound queue after drain = %d, want 0", remaining)
	}
}

// buildGuestARPRequest constructs an Ethernet ARP request from the guest asking
// who owns targetIP.
func buildGuestARPRequest(srcMAC [6]byte, senderIP, targetIP [4]byte) []byte {
	// Build the Ethernet broadcast header for a guest ARP request.
	frame := make([]byte, 42)
	for i := range frame[0:6] {
		frame[i] = 0xff // broadcast destination
	}
	copy(frame[6:12], srcMAC[:])
	binary.BigEndian.PutUint16(frame[12:14], 0x0806) // ARP ethertype

	// Encode the ARP protocol header for an Ethernet IPv4 request.
	binary.BigEndian.PutUint16(frame[14:16], 1)      // htype: Ethernet
	binary.BigEndian.PutUint16(frame[16:18], 0x0800) // ptype: IPv4
	frame[18] = 6                                    // hlen
	frame[19] = 4                                    // plen
	binary.BigEndian.PutUint16(frame[20:22], 1)      // oper: request

	// Encode the guest sender address and requested gateway address.
	copy(frame[22:28], srcMAC[:])
	copy(frame[28:32], senderIP[:])
	copy(frame[38:42], targetIP[:])

	return frame
}

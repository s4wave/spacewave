package v86_wazero

import (
	"context"
	"net"
	"net/netip"

	"github.com/s4wave/spacewave/forge/lib/v86/wazero/usernet"
	"github.com/sirupsen/logrus"
)

// defaultGuestMAC is the guest NIC address used when NetworkConfig leaves
// GuestMAC zero. The locally-administered bit (0x02) is set so it never
// collides with a real vendor OUI.
var defaultGuestMAC = [6]byte{0x02, 0x22, 0x15, 0x00, 0x00, 0x01}

// NetworkConfig enables guest networking by attaching an NE2000 NIC backed by
// the in-process usermode stack. A zero value is valid: usernet fills the
// guest/gateway/DNS addresses, dialer, and resolver with their defaults, and a
// zero GuestMAC becomes defaultGuestMAC.
type NetworkConfig struct {
	// GuestMAC is the NE2000 hardware address; zero selects defaultGuestMAC.
	GuestMAC [6]byte
	// GuestIP is the IPv4 address leased to the guest over DHCP.
	GuestIP netip.Addr
	// GatewayIP is the router address the stack owns.
	GatewayIP netip.Addr
	// Netmask is the DHCP subnet mask.
	Netmask netip.Addr
	// DNSServer is the DHCP-advertised resolver address proxied by the stack.
	DNSServer netip.Addr
	// Dialer opens host TCP and UDP sockets; nil uses a default net.Dialer.
	Dialer usernet.ContextDialer
	// Resolver answers DNS proxy questions; nil uses net.DefaultResolver.
	Resolver *net.Resolver
	// Logger receives host-socket error logs; nil is replaced inside the stack.
	Logger *logrus.Entry
}

// toUsernet builds the usernet stack configuration for the resolved guest MAC.
func (c NetworkConfig) toUsernet(mac [6]byte) usernet.Config {
	return usernet.Config{
		GuestMAC:  mac,
		GuestIP:   c.GuestIP,
		GatewayIP: c.GatewayIP,
		Netmask:   c.Netmask,
		DNSServer: c.DNSServer,
		Dialer:    c.Dialer,
		Resolver:  c.Resolver,
	}
}

// registerNetworking attaches the NE2000 device and usermode stack. The stack's
// inbound sink queues host-origin frames on the device (thread-safe) and pokes
// the wake channel so RunSerialConsole drains them on the next CPU tick instead
// of waiting on the emulator idle timer. Guest transmits flow back out through
// SetOutbound on the CPU goroutine.
func (h *HostRuntime) registerNetworking(ctx context.Context, cfg NetworkConfig) {
	if h.pci == nil {
		return
	}
	mac := cfg.GuestMAC
	if mac == ([6]byte{}) {
		mac = defaultGuestMAC
	}
	dev := h.registerNE2K(ctx, 0, mac)
	wake := make(chan struct{}, 1)
	stack := usernet.New(cfg.toUsernet(mac), func(frame []byte) {
		dev.QueueInbound(frame)
		select {
		case wake <- struct{}{}:
		default:
		}
	}, cfg.Logger)
	dev.SetOutbound(stack.HandleOutbound)
	h.ne2k = dev
	h.network = stack
	h.netWake = wake
}

// drainNetwork delivers queued host-origin frames into the receive ring. It is
// a no-op when networking is disabled and must run on the CPU goroutine.
func (h *HostRuntime) drainNetwork(ctx context.Context) {
	if h.ne2k != nil {
		h.ne2k.DrainInbound(ctx)
	}
}

// netWakeCh returns the inbound-frame wake channel, or nil when networking is
// off so a select case on it blocks forever.
func (h *HostRuntime) netWakeCh() <-chan struct{} {
	return h.netWake
}

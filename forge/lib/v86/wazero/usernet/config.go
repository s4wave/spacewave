package usernet

import (
	"context"
	"net"
	"net/netip"
)

// ContextDialer dials host sockets with a caller-owned context.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Config is the usermode network configuration.
type Config struct {
	// guestMAC is the guest NIC MAC address
	GuestMAC [6]byte
	// guestIP is the leased IPv4 address advertised to the guest
	GuestIP netip.Addr
	// gatewayIP is the router IPv4 address owned by the stack
	GatewayIP netip.Addr
	// netmask is the DHCP subnet mask
	Netmask netip.Addr
	// dnsServer is the DHCP DNS server address proxied by the stack
	DNSServer netip.Addr
	// dialer opens host TCP and UDP sockets
	Dialer ContextDialer
	// resolver resolves DNS questions for the DNS proxy
	Resolver *net.Resolver
}

// withDefaults fills unset configuration values: gateway, netmask, DNS,
func (c Config) withDefaults() Config {
	if !c.GuestIP.IsValid() {
		c.GuestIP = netip.MustParseAddr("10.0.2.15")
	}
	if !c.GatewayIP.IsValid() {
		c.GatewayIP = netip.MustParseAddr("10.0.2.2")
	}
	if !c.Netmask.IsValid() {
		c.Netmask = netip.MustParseAddr("255.255.255.0")
	}
	if !c.DNSServer.IsValid() {
		c.DNSServer = c.GatewayIP
	}
	if c.Dialer == nil {
		c.Dialer = &net.Dialer{}
	}
	if c.Resolver == nil {
		c.Resolver = net.DefaultResolver
	}
	return c
}

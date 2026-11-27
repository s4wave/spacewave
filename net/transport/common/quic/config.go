package transport_quic

import (
	"crypto/tls"
	"math"
	"time"

	quic "github.com/quic-go/quic-go"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
)

// minPacketSize is the smallest packet size QUIC allows.
const minPacketSize = 1200

// BuildQuicConfig constructs the quic config.
func BuildQuicConfig(opts *Opts) *quic.Config {
	// Resolve the configured idle timeout.
	maxIdleTimeout := time.Second * 10
	if ntDur := opts.GetMaxIdleTimeoutDur(); ntDur != "" {
		nt, err := time.ParseDuration(ntDur)
		if err == nil && nt > time.Duration(0) {
			maxIdleTimeout = nt
		}
	}

	// Resolve the configured incoming stream limit.
	maxIncStreams := 100000
	if mis := opts.GetMaxIncomingStreams(); mis > 0 {
		maxIncStreams = int(mis)
	}

	// Resolve the keepalive period from the timeout and options.
	keepAlivePeriod := maxIdleTimeout / 2
	if opts.GetDisableKeepAlive() {
		keepAlivePeriod = 0
	} else if keepAliveDur := opts.GetKeepAliveDur(); keepAliveDur != "" {
		kaDur, err := time.ParseDuration(keepAliveDur)
		if err == nil && kaDur > time.Duration(0) {
			keepAlivePeriod = kaDur
		}
	}

	// Start at the smallest QUIC packet, which fits every IP path, including
	// tunnels with a 1280 byte MTU such as Tailscale, where quic-go's 1280 byte
	// default leaves no room for the IP and UDP headers. Path MTU discovery
	// grows the size from there.
	initialPacketSize := uint16(min(opts.GetInitialPacketSize(), math.MaxUint16))
	if initialPacketSize == 0 {
		initialPacketSize = minPacketSize
	}

	// Build the QUIC configuration from the resolved limits.
	return &quic.Config{
		// Enable datagrams unless the options disable them. Unreliable message
		// streams use them, and WebTransport requires them.
		EnableDatagrams:         !opts.GetDisableDatagrams(),
		KeepAlivePeriod:         keepAlivePeriod,
		DisablePathMTUDiscovery: opts.GetDisablePathMtuDiscovery(),
		InitialPacketSize:       initialPacketSize,

		MaxIdleTimeout:        maxIdleTimeout,
		MaxIncomingStreams:    int64(maxIncStreams),
		MaxIncomingUniStreams: -1, // disable unidirectional streams
	}
}

// BuildIncomingTlsConf builds the tls config for incoming conns.
//
// rpeer can be empty to indicate accepting any remote peer.
func BuildIncomingTlsConf(identity *p2ptls.Identity, rpeer peer.ID) *tls.Config {
	// Configure ALPN and the peer-aware TLS callback.
	var tlsConf tls.Config
	tlsConf.NextProtos = []string{Alpn}
	tlsConf.GetConfigForClient = func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
		// Resolve the peer-specific TLS configuration for each client.
		conf, _ := identity.ConfigForPeer(rpeer)

		// Require the QUIC ALPN and disable session tickets.
		conf.NextProtos = []string{Alpn}

		// TODO: https://github.com/golang/go/issues/60506
		conf.SessionTicketsDisabled = true
		return conf, nil
	}

	// Disable session tickets on the listener configuration.
	// TODO: https://github.com/golang/go/issues/60506
	tlsConf.SessionTicketsDisabled = true
	return &tlsConf
}

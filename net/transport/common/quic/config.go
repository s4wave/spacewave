package transport_quic

import (
	"context"
	"crypto/tls"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlogwriter"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// BuildQuicConfig constructs the QUIC config without verbose packet logging.
func BuildQuicConfig(opts *Opts) *quic.Config {
	return buildQuicConfig(opts, nil)
}

// BuildQuicConfigWithLogger constructs the QUIC config and emits packet and
// frame events through le when opts enables verbose logging.
func BuildQuicConfigWithLogger(opts *Opts, le *logrus.Entry) *quic.Config {
	return buildQuicConfig(opts, le)
}

func buildQuicConfig(opts *Opts, le *logrus.Entry) *quic.Config {
	maxIdleTimeout := time.Second * 10
	if ntDur := opts.GetMaxIdleTimeoutDur(); ntDur != "" {
		nt, err := time.ParseDuration(ntDur)
		if err == nil && nt > time.Duration(0) {
			maxIdleTimeout = nt
		}
	}

	maxIncStreams := 100000
	if mis := opts.GetMaxIncomingStreams(); mis > 0 {
		maxIncStreams = int(mis)
	}

	keepAlivePeriod := maxIdleTimeout / 2
	if opts.GetDisableKeepAlive() {
		keepAlivePeriod = 0
	} else if keepAliveDur := opts.GetKeepAliveDur(); keepAliveDur != "" {
		kaDur, err := time.ParseDuration(keepAliveDur)
		if err == nil && kaDur > time.Duration(0) {
			keepAlivePeriod = kaDur
		}
	}

	config := &quic.Config{
		// We don't use datagrams (yet), but this is necessary for WebTransport
		EnableDatagrams:         !opts.GetDisableDatagrams(),
		KeepAlivePeriod:         keepAlivePeriod,
		DisablePathMTUDiscovery: opts.GetDisablePathMtuDiscovery(),

		MaxIdleTimeout:        maxIdleTimeout,
		MaxIncomingStreams:    int64(maxIncStreams),
		MaxIncomingUniStreams: -1, // disable unidirectional streams
	}
	if opts.GetVerbose() {
		config.Tracer = func(
			_ context.Context,
			isClient bool,
			connID quic.ConnectionID,
		) qlogwriter.Trace {
			return newLogQlogTrace(le, isClient, connID)
		}
	}
	return config
}

// BuildIncomingTlsConf builds the tls config for incoming conns.
//
// rpeer can be empty to indicate accepting any remote peer.
func BuildIncomingTlsConf(identity *p2ptls.Identity, rpeer peer.ID) *tls.Config {
	var tlsConf tls.Config
	tlsConf.NextProtos = []string{Alpn}
	tlsConf.GetConfigForClient = func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
		// note: if rpeer is empty, allows any incoming peer id.
		conf, _ := identity.ConfigForPeer(rpeer)
		conf.NextProtos = []string{Alpn}
		// TODO: https://github.com/golang/go/issues/60506
		conf.SessionTicketsDisabled = true
		return conf, nil
	}

	// TODO: https://github.com/golang/go/issues/60506
	tlsConf.SessionTicketsDisabled = true
	return &tlsConf
}

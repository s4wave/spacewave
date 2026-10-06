//go:build !js

package webrtc_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pion/logging"
	"github.com/pion/transport/v5/vnet"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// iceNetwork is a vnet that carries the ICE data path of the transports a test
// starts. Each transport takes the next unused address, and crashing a
// transport silences its address as a crashed process does.
type iceNetwork struct {
	// mtx guards dead.
	mtx sync.Mutex
	// dead holds the hosts of crashed transports.
	dead map[string]bool
	// nets holds one network for each transport the test starts.
	nets []*vnet.Net
	// next is the index of the first unused network.
	next int
}

// newICENetwork starts a router with room for count transports. It stops with
// the test.
func newICENetwork(t *testing.T, count int) *iceNetwork {
	// Route through a filter that drops traffic of crashed transports.
	t.Helper()
	n := &iceNetwork{dead: make(map[string]bool)}
	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "10.0.0.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	router.AddChunkFilter(func(c vnet.Chunk) bool {
		n.mtx.Lock()
		defer n.mtx.Unlock()
		return !n.dead[addrHost(c.SourceAddr())] && !n.dead[addrHost(c.DestinationAddr())]
	})

	// Attach every address before the router starts: vnet routes only the
	// networks attached by then.
	for i := range count {
		iceNet, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{n.ip(i)}})
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := router.AddNet(iceNet); err != nil {
			t.Fatal(err.Error())
		}
		n.nets = append(n.nets, iceNet)
	}
	if err := router.Start(); err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() { _ = router.Stop() })
	return n
}

// ip returns the address of the network at index i.
func (n *iceNetwork) ip(i int) string {
	return fmt.Sprintf("10.0.0.%d", 10+i)
}

// startTransport runs a WebRTC transport on b with the next unused address and
// returns a function that crashes it. Crashing silences the address before
// stopping the transport, so the remote peer hears nothing from it again.
func (n *iceNetwork) startTransport(
	ctx context.Context,
	t *testing.T,
	le *logrus.Entry,
	b bus.Bus,
	conf *webrtc.Config,
) func() {
	// Take the next unused address.
	t.Helper()
	i := n.next
	n.next++
	ip := n.ip(i)

	// Construct and run the transport controller on the peer bus.
	ctrl, err := webrtc.NewFactory(b, webrtc.WithICENet(n.nets[i])).Construct(
		ctx,
		conf,
		controller.ConstructOpts{Logger: le.WithField("transport-ip", ip)},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	tptCtx, tptCancel := context.WithCancel(ctx)
	go func() { _ = b.ExecuteController(tptCtx, ctrl) }()

	return func() {
		n.mtx.Lock()
		n.dead[ip] = true
		n.mtx.Unlock()
		tptCancel()
	}
}

// addrHost returns the host part of a vnet chunk address.
func addrHost(addr net.Addr) string {
	host, _, _ := net.SplitHostPort(addr.String())
	return host
}

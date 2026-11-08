//go:build !js

package webrtc

import (
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/pion/logging"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/vnet"
	"github.com/pion/webrtc/v4"
)

// namedNet lists each of a vnet's addresses on its own named interface.
type namedNet struct {
	*vnet.Net
	// ifaces are the listed interfaces.
	ifaces []*transport.Interface
}

// Interfaces returns the named interfaces.
func (n *namedNet) Interfaces() ([]*transport.Interface, error) { return n.ifaces, nil }

// InterfaceByName returns the named interface.
func (n *namedNet) InterfaceByName(name string) (*transport.Interface, error) {
	for _, ifc := range n.ifaces {
		if ifc.Name == name {
			return ifc, nil
		}
	}
	return nil, transport.ErrInterfaceNotFound
}

// TestGatherSkipsContainerInterfaces gathers on a host whose interfaces
// include docker, veth and bridge ones, and checks they are skipped by default
// and gathered when the config asks for every interface.
func TestGatherSkipsContainerInterfaces(t *testing.T) {
	// One address per interface, each named as a host running containers does.
	names := []string{"eth0", "docker0", "veth1a2b", "br-5f3c"}
	ips := []string{"10.0.0.10", "10.0.0.11", "10.0.0.12", "10.0.0.13"}
	router, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: "10.0.0.0/24", LoggerFactory: logging.NewDefaultLoggerFactory()})
	if err != nil {
		t.Fatal(err)
	}
	vn, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: ips})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.AddNet(vn); err != nil {
		t.Fatal(err)
	}
	if err := router.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = router.Stop() })
	nn := &namedNet{Net: vn}
	for i, name := range names {
		ifc := transport.NewInterface(net.Interface{Index: i + 1, Name: name, MTU: 1500, Flags: net.FlagUp | net.FlagMulticast})
		ifc.AddAddress(&net.IPNet{IP: net.ParseIP(ips[i]).To4(), Mask: net.CIDRMask(24, 32)})
		nn.ifaces = append(nn.ifaces, ifc)
	}

	// gather returns the distinct candidate addresses for one config.
	gather := func(conf *Config) []string {
		se := webrtc.SettingEngine{}
		applyICEOptions(&se, &options{iceNet: nn}, conf)
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
		pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			t.Fatal(err)
		}
		defer pc.Close()
		if _, err := pc.CreateDataChannel("probe", nil); err != nil {
			t.Fatal(err)
		}
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			t.Fatal(err)
		}
		done := webrtc.GatheringCompletePromise(pc)
		if err := pc.SetLocalDescription(offer); err != nil {
			t.Fatal(err)
		}
		<-done
		var addrs []string
		for line := range strings.SplitSeq(pc.LocalDescription().SDP, "\n") {
			if f := strings.Fields(line); len(f) > 4 && strings.HasPrefix(f[0], "a=candidate:") {
				addrs = append(addrs, f[4])
			}
		}
		slices.Sort(addrs)
		return slices.Compact(addrs)
	}

	// By default only the host's own interface gathers.
	if got := gather(&Config{}); !slices.Equal(got, []string{"10.0.0.10"}) {
		t.Fatalf("default gathered %v", got)
	}

	// Asking for every interface restores them.
	want := slices.Clone(ips)
	slices.Sort(want)
	if got := gather(&Config{GatherAllInterfaces: true}); !slices.Equal(got, want) {
		t.Fatalf("all interfaces gathered %v", got)
	}
}

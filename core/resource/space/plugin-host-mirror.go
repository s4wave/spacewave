package resource_space

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// spacePluginHostMirror projects daemon plugin hosts onto one Space runtime bus.
type spacePluginHostMirror struct {
	bcast broadcast.Broadcast
	hosts []plugin_host.PluginHost
	insts map[directive.Instance]struct{}
	ready bool
}

func newSpacePluginHostMirror() *spacePluginHostMirror {
	return &spacePluginHostMirror{insts: make(map[directive.Instance]struct{})}
}

func (m *spacePluginHostMirror) GetControllerInfo() *controller.Info {
	return controller.NewInfo("space/plugin-host-mirror", controller.MustParseVersion("0.0.1"), "Space plugin host mirror")
}

func (m *spacePluginHostMirror) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (m *spacePluginHostMirror) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(plugin_host.LookupPluginHost)
	if !ok {
		return nil, nil
	}

	var hosts []plugin_host.PluginHost
	m.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		platformIDs := dir.LookupPluginHostPlatformIDs()
		for _, host := range m.hosts {
			if len(platformIDs) == 0 || slices.Contains(platformIDs, host.GetPlatformId()) {
				hosts = append(hosts, host)
			}
		}
		m.insts[inst] = struct{}{}
	})
	release := inst.AddDisposeCallback(func() {
		m.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			delete(m.insts, inst)
		})
	})
	_ = release

	return directive.R(directive.NewValueResolver(hosts), nil)
}

func (m *spacePluginHostMirror) SetHosts(hosts []plugin_host.PluginHost) {
	var terminal []directive.Instance
	m.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		m.hosts = slices.Clone(hosts)
		if m.ready {
			terminal = make([]directive.Instance, 0, len(m.insts))
			for inst := range m.insts {
				terminal = append(terminal, inst)
			}
		} else {
			m.ready = true
		}
		broadcast()
	})
	for _, inst := range terminal {
		inst.Close()
	}
}

func (m *spacePluginHostMirror) Close() error { return nil }

// _ is a type assertion
var _ controller.Controller = (*spacePluginHostMirror)(nil)

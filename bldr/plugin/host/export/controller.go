package plugin_host_export

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// Controller resolves LookupPluginHost and LookupRoot on a plugin's bus with
// proxies for the plugin hosts its host exports.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// client reaches the host's HostExport service
	client SRPCHostExportClient
	// hostsCtr holds the current proxies, nil until the first host report.
	// Each hostSet is immutable once published.
	hostsCtr *ccontainer.CContainer[*hostSet]
}

// hostSet is a snapshot of the proxies, one per platform ID.
type hostSet struct {
	hosts []*proxyHost
}

// get returns the proxy for platformID, or nil.
func (s *hostSet) get(platformID string) *proxyHost {
	if s == nil {
		return nil
	}
	i := slices.IndexFunc(s.hosts, func(h *proxyHost) bool { return h.platformID == platformID })
	if i < 0 {
		return nil
	}
	return s.hosts[i]
}

// NewController constructs the controller on the plugin bus b.
func NewController(le *logrus.Entry, b bus.Bus) *Controller {
	return &Controller{
		le: le,
		client: NewSRPCHostExportClientWithServiceID(
			bifrost_rpc.NewBusClient(b),
			bldr_plugin.HostServiceIDPrefix+SRPCHostExportServiceID,
		),
		hostsCtr: ccontainer.NewCContainer[*hostSet](nil),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "exported plugin host proxies")
}

// Execute follows the exported hosts. Proxies stay stable across reports so
// the scheduler keeps its running plugins on retained platforms.
func (c *Controller) Execute(ctx context.Context) error {
	strm, err := c.client.WatchHosts(ctx, &WatchHostsRequest{})
	if err != nil {
		return err
	}
	defer strm.Close()
	for {
		resp, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "watch exported plugin hosts")
		}
		c.hostsCtr.SwapValue(func(prev *hostSet) *hostSet {
			next := &hostSet{}
			for _, platformID := range resp.GetPlatformIds() {
				host := prev.get(platformID)
				if host == nil {
					host = newProxyHost(c.client, platformID)
				}
				next.hosts = append(next.hosts, host)
			}
			return next
		})
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case plugin_host.LookupPluginHost:
		return directive.R(c.resolve(d.LookupPluginHostPlatformIDs(), func(h *proxyHost) directive.Value {
			return plugin_host.LookupPluginHostValue(h)
		}), nil)
	case plugin_host_root.LookupRoot:
		return directive.R(c.resolve(d.LookupRootPlatformIDs(), func(h *proxyHost) directive.Value {
			return h.root
		}), nil)
	}
	return nil, nil
}

// resolve follows the proxies matching platformIDs, or all when empty, and
// resolves value for each.
func (c *Controller) resolve(platformIDs []string, value func(*proxyHost) directive.Value) directive.Resolver {
	return directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		valueIDs := make(map[*proxyHost]uint32)
		var set *hostSet
		for {
			var err error
			set, err = c.hostsCtr.WaitValueChange(ctx, set, nil)
			if err != nil {
				return err
			}
			var current []*proxyHost
			for _, host := range set.hosts {
				if len(platformIDs) == 0 || slices.Contains(platformIDs, host.platformID) {
					current = append(current, host)
				}
			}
			for host, id := range valueIDs {
				if !slices.Contains(current, host) {
					handler.RemoveValue(id)
					delete(valueIDs, host)
				}
			}
			for _, host := range current {
				if _, ok := valueIDs[host]; ok {
					continue
				}
				if id, accepted := handler.AddValue(value(host)); accepted {
					valueIDs[host] = id
				}
			}
			handler.MarkIdle(true)
		}
	})
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)

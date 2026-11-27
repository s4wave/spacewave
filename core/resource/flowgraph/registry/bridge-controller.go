package resource_flowgraph_registry

import (
	"context"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// BridgeController publishes compile proxies for registered plugin node types,
// and the factories that run their plugin configs inside the plugin that
// compiled them.
type BridgeController struct {
	// b resolves the compiled controller config constructors.
	b bus.Bus
	// registry retains compiler capabilities and publishes registration changes.
	registry *RegistryResource
}

// NewBridgeController constructs the bridge for a daemon's node type registry.
func NewBridgeController(b bus.Bus, registry *RegistryResource) *BridgeController {
	return &BridgeController{b: b, registry: registry}
}

// GetControllerInfo describes the node type bridge.
func (c *BridgeController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("resource/flowgraph-node-types", controller.MustParseVersion("0.0.1"), "plugin node type compile proxies")
}

// Execute leaves registration watches to the lookup resolvers.
func (c *BridgeController) Execute(context.Context) error {
	return nil
}

// HandleDirective follows registration arrival, withdrawal and replacement
// for node type lookups and for the factories of compiled plugin configs.
func (c *BridgeController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Match standing demand even when its plugin has not registered yet.
	switch dir := di.GetDirective().(type) {
	case s4wave_flowgraph.LookupFlowgraphNodeType:
		engineID, typeID := dir.LookupFlowgraphNodeTypeEngineID(), dir.LookupFlowgraphNodeTypeID()
		return c.watch(func() (*nodeTypeRegistration, <-chan struct{}) {
			return c.registry.watchRegistration(engineID, typeID)
		}, func(reg *nodeTypeRegistration) directive.Value {
			return newNodeTypeProxy(c.b, reg)
		})
	case bifrost_rpc.LookupRpcService:
		route, ok := strings.CutPrefix(dir.LookupRpcServerID(), "flowgraph-plugin/")
		if !ok {
			return nil, nil
		}
		id, serverID, ok := strings.Cut(route, "/")
		regID, err := strconv.ParseUint(id, 10, 32)
		if !ok || err != nil || serverID == "" {
			return nil, nil
		}
		return c.watch(func() (*nodeTypeRegistration, <-chan struct{}) {
			return c.registry.watchRegistrationByID(uint32(regID)) //nolint:gosec // ParseUint above limits the value to 32 bits.
		}, func(reg *nodeTypeRegistration) directive.Value {
			return &pluginPortInvoker{reg: reg, serverID: serverID}
		})
	case resolver.LoadFactoryByConfig:
		conf, ok := dir.LoadFactoryByConfig().(*pluginConfig)
		if !ok || conf.reg == nil {
			return nil, nil
		}
		return c.watch(func() (*nodeTypeRegistration, <-chan struct{}) {
			return c.registry.watchRegistrationByID(conf.reg.id)
		}, func(reg *nodeTypeRegistration) directive.Value {
			return &pluginFactory{configID: conf.configID, reg: reg}
		})
	}
	return nil, nil
}

// watch publishes the value for the registration that lookup returns,
// replacing the value when the registration changes and withdrawing the value
// when no registration remains.
func (c *BridgeController) watch(
	lookup func() (*nodeTypeRegistration, <-chan struct{}),
	value func(*nodeTypeRegistration) directive.Value,
) ([]directive.Resolver, error) {
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		var current *nodeTypeRegistration
		var valueID uint32
		for {
			// Read the registration and its change notification together.
			next, wait := lookup()

			// Withdraw the old value before publishing a replacement.
			if next != current {
				if valueID != 0 {
					handler.RemoveValue(valueID)
					valueID = 0
				}
				current = next
				if next != nil {
					valueID, _ = handler.AddValue(value(next))
				}
			}

			// Keep unresolved demand alive until the registration changes.
			handler.MarkIdle(true)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wait:
			}
			handler.MarkIdle(false)
		}
	}), nil)
}

// Close releases no state because registration Resources retain the compilers.
func (c *BridgeController) Close() error {
	return nil
}

// _ is a type assertion.
var _ controller.Controller = (*BridgeController)(nil)

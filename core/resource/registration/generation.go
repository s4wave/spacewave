package registration

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	sdk_registration "github.com/s4wave/spacewave/sdk/plugin/registration"
)

// generationKey carries a caller-owned registration scope through existing RPCs.
type generationKey struct{}

// binding identifies independently admitted installations of a plugin family.
type binding struct {
	pluginID    string
	instanceKey string
}

// Generation retains one plugin executable's private or admitted registrations.
// Its state is protected by the Registry broadcast lock.
type Generation struct {
	// registry owns admission across all participating registries.
	registry *Registry
	// binding identifies the family and installation replaced during activation.
	binding
	// manifestRoot identifies the implementation served by this generation.
	manifestRoot string
	// root serves the existing registration methods.
	root srpc.Invoker
	// mux serves this generation's activation method.
	mux srpc.Mux
	// activated prevents a retired generation from being admitted again.
	activated bool
	// previous keeps the last working group recoverable until its worker retires.
	previous *Generation
	// closed records release of the scope Resource.
	closed bool
}

// ManifestRoot returns the exact implementation registered by this generation.
func (g *Generation) ManifestRoot() string {
	return g.manifestRoot
}

// FromContext returns the private registration scope, or nil for ordinary
// registrations. A named plugin must match the scope's family.
func FromContext(ctx context.Context, pluginID string) (*Generation, error) {
	generation, _ := ctx.Value(generationKey{}).(*Generation)
	if generation != nil && pluginID != "" && generation.pluginID != pluginID {
		return nil, errors.New("registration belongs to another plugin family")
	}
	return generation, nil
}

// Activate publishes the whole generation in one critical section. Retrying the
// active generation succeeds; a retired or closed generation cannot supersede it.
func (g *Generation) Activate(ctx context.Context, req *sdk_registration.ActivateRequest) (*sdk_registration.ActivateResponse, error) {
	var err error
	g.registry.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Do not publish after the request or owning Resource was canceled.
		if err = ctx.Err(); err != nil {
			return
		}
		if g.closed || g.activated && g.registry.active[g.binding] != g {
			err = errors.New("plugin generation has been retired")
			return
		}
		if g.registry.active[g.binding] == g {
			return
		}

		// Change visibility before notifying every registry that exposes the group.
		g.previous = g.registry.active[g.binding]
		g.activated = true
		g.registry.active[g.binding] = g
		for _, changed := range g.registry.changed {
			changed()
		}
		broadcast()
	})
	if err != nil {
		return nil, err
	}
	return &sdk_registration.ActivateResponse{}, nil
}

// Close hides this generation. During handover, a failed candidate restores the
// still-retained previous worker; once that worker retires it cannot reappear.
func (g *Generation) Close() {
	g.registry.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if g.closed {
			return
		}
		g.closed = true
		current := g.registry.active[g.binding]
		if current != g {
			// Unlink a retired group from the short handover chain.
			for next := current; next != nil; next = next.previous {
				if next.previous == g {
					next.previous = g.previous
					break
				}
			}
			g.previous = nil
			return
		}
		delete(g.registry.active, g.binding)
		if g.previous != nil {
			g.registry.active[g.binding] = g.previous
		}
		g.previous = nil
		for _, changed := range g.registry.changed {
			changed()
		}
		broadcast()
	})
}

// InvokeMethod binds registration calls to this private generation. Other root
// services are deliberately absent from a registration scope.
func (g *Generation) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID == sdk_registration.SRPCGenerationServiceServiceID {
		return g.mux.InvokeMethod(serviceID, methodID, strm)
	}
	switch serviceID {
	case "s4wave.objecttype.registry.ObjectTypeRegistryResourceService",
		"s4wave.worldop.registry.WorldOpRegistryResourceService",
		"s4wave.viewer.registry.ViewerRegistryResourceService",
		"s4wave.quickstart.registry.QuickstartRegistryResourceService",
		"s4wave.wizard.ObjectWizardRegistryResourceService":
		ctx := context.WithValue(strm.Context(), generationKey{}, g)
		return g.root.InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(strm, ctx))
	default:
		return false, nil
	}
}

var _ srpc.Invoker = (*Generation)(nil)

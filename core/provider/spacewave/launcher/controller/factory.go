package spacewave_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/configresolve"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// Factory constructs a launcher controller factory.
type Factory struct {
	// bus is the controller bus
	bus bus.Bus
}

// NewFactory builds the factory.
func NewFactory(bus bus.Bus) *Factory {
	return &Factory{bus: bus}
}

// GetConfigID returns the configuration ID for the controller.
func (t *Factory) GetConfigID() string {
	return ConfigID
}

// GetControllerID returns the unique ID for the controller.
func (t *Factory) GetControllerID() string {
	return ControllerID
}

// ConstructConfig constructs an instance of the controller configuration.
func (t *Factory) ConstructConfig() config.Config {
	return &Config{}
}

// Construct constructs the associated controller given configuration.
func (t *Factory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	le := opts.GetLogger()
	cc := conf.(*Config)

	if err := cc.Validate(); err != nil {
		return nil, err
	}
	endpointFetchDisabled := cc.GetDisableEndpointFetch()

	distPeerIDs, err := ResolveDistPeerIDs(cc)
	if err != nil {
		return nil, errors.Wrap(err, "resolve dist_peer_ids")
	}
	if len(distPeerIDs) == 0 {
		return nil, errors.New("dist_peer_ids: no peer IDs from config or build-time embedding")
	}

	endpoints, err := ResolveEndpoints(cc)
	if err != nil {
		return nil, errors.Wrap(err, "resolve endpoints")
	}
	if len(endpoints) == 0 && !endpointFetchDisabled {
		return nil, errors.New("endpoints: no DistConfig endpoints from config or build-time embedding")
	}

	return NewController(le, t.bus, cc, distPeerIDs, endpoints), nil
}

// ResolveEndpoints returns Config.Endpoints when provided, otherwise the
// production build-time defaults. Config values replace defaults so
// release-owned overlays do not inherit production endpoints.
func ResolveEndpoints(conf *Config) ([]*HttpEndpoint, error) {
	_, endps, err := conf.ParseEndpointURLs()
	if err != nil {
		return nil, err
	}
	configURLs := make([]string, 0, len(endps))
	for _, endp := range endps {
		configURLs = append(configURLs, endp.GetUrl())
	}
	resolved, err := configresolve.ResolveEndpoints(
		conf.GetDisableEndpointFetch(),
		configURLs,
		BuildTimeDistConfigEndpoints,
	)
	if err != nil {
		return nil, err
	}
	endpoints := make([]*HttpEndpoint, 0, len(resolved))
	for _, u := range resolved {
		endpoints = append(endpoints, &HttpEndpoint{Url: u})
	}
	return endpoints, nil
}

// ResolveDistPeerIDs returns Config.DistPeerIds when provided, otherwise the
// production build-time defaults. Config values replace defaults so
// release-owned overlays do not inherit production signer trust.
func ResolveDistPeerIDs(conf *Config) ([]peer.ID, error) {
	ids := configresolve.ResolveDistPeerIDs(conf.GetDistPeerIds(), BuildTimeDistPeerIDs)
	parsed, err := confparse.ParsePeerIDs(ids, false)
	if err != nil {
		if len(conf.GetDistPeerIds()) == 0 {
			return nil, errors.Wrap(err, "build-time peer ids")
		}
		return nil, err
	}
	return parsed, nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))

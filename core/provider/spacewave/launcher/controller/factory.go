package spacewave_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
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
	if len(endpoints) == 0 {
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
	configEndps := dedupEndpoints(endps)
	if len(configEndps) != 0 {
		return configEndps, nil
	}

	fallback := make([]*HttpEndpoint, 0, len(BuildTimeDistConfigEndpoints))
	seen := make(map[string]struct{}, len(BuildTimeDistConfigEndpoints))
	for _, u := range BuildTimeDistConfigEndpoints {
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		if _, err := confparse.ParseURL(u); err != nil {
			return nil, errors.Wrapf(err, "build-time endpoint %q", u)
		}
		seen[u] = struct{}{}
		fallback = append(fallback, &HttpEndpoint{Url: u})
	}
	return fallback, nil
}

func dedupEndpoints(endps []*HttpEndpoint) []*HttpEndpoint {
	deduped := make([]*HttpEndpoint, 0, len(endps))
	seen := make(map[string]struct{}, len(endps))
	for _, endp := range endps {
		u := endp.GetUrl()
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		deduped = append(deduped, endp)
	}
	return deduped
}

// ResolveDistPeerIDs returns Config.DistPeerIds when provided, otherwise the
// production build-time defaults. Config values replace defaults so
// release-owned overlays do not inherit production signer trust.
func ResolveDistPeerIDs(conf *Config) ([]peer.ID, error) {
	configIDs, err := conf.ParseDistPeerIds()
	if err != nil {
		return nil, err
	}
	if len(configIDs) != 0 {
		return dedupPeerIDs(configIDs), nil
	}
	buildTimeIDs, err := confparse.ParsePeerIDs(BuildTimeDistPeerIDs, false)
	if err != nil {
		return nil, errors.Wrap(err, "build-time peer ids")
	}
	return dedupPeerIDs(buildTimeIDs), nil
}

func dedupPeerIDs(ids []peer.ID) []peer.ID {
	deduped := make([]peer.ID, 0, len(ids))
	seen := make(map[peer.ID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	return deduped
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() controller.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))

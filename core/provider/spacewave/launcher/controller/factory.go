package spacewave_launcher_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver/v4"
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

// ResolveEndpoints merges Config.Endpoints with BuildTimeDistConfigEndpoints
// and deduplicates by URL. Build-time entries are appended after config
// entries; both sources are URL-parsed to filter invalid entries.
func ResolveEndpoints(conf *Config) ([]*HttpEndpoint, error) {
	endps := conf.CloneSortEndpoints()
	merged := make([]*HttpEndpoint, 0, len(endps)+len(BuildTimeDistConfigEndpoints))
	seen := make(map[string]struct{}, len(endps)+len(BuildTimeDistConfigEndpoints))
	for _, endp := range endps {
		u := endp.GetUrl()
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		merged = append(merged, endp)
	}
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
		merged = append(merged, &HttpEndpoint{Url: u})
	}
	return merged, nil
}

// ResolveDistPeerIDs merges Config.DistPeerIds with BuildTimeDistPeerIDs and
// deduplicates the result.
func ResolveDistPeerIDs(conf *Config) ([]peer.ID, error) {
	configIDs, err := conf.ParseDistPeerIds()
	if err != nil {
		return nil, err
	}
	buildTimeIDs, err := confparse.ParsePeerIDs(BuildTimeDistPeerIDs, false)
	if err != nil {
		return nil, errors.Wrap(err, "build-time peer ids")
	}
	merged := make([]peer.ID, 0, len(configIDs)+len(buildTimeIDs))
	seen := make(map[peer.ID]struct{}, len(configIDs)+len(buildTimeIDs))
	for _, id := range append(configIDs, buildTimeIDs...) {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}
	return merged, nil
}

// GetVersion returns the version of this controller.
func (t *Factory) GetVersion() semver.Version {
	return Version
}

// _ is a type assertion
var _ controller.Factory = ((*Factory)(nil))

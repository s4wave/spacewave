package rpc_volume_client

import (
	"context"
	"regexp"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/aperturerobotics/hydra/bucket"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the rpc volume client controller id.
const ControllerID = "hydra/volume/rpc/client"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Controller implements the rpc volume client.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// cc is controller config
	cc *Config
	// matchVolumeIdRe is the regexp to match volume ids
	// if nil, match any
	matchVolumeIdRe *regexp.Regexp
	// proxyVolumes is the list of proxy volume trackers.
	proxyVolumes *keyed.KeyedRefCount[*proxyVolumeTracker]
}

// NewController constructs a new rpc volume server.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	cc *Config,
) (*Controller, error) {
	volumeIDRe, err := cc.ParseVolumeIdRe()
	if err != nil {
		return nil, err
	}
	c := &Controller{
		le:              le,
		bus:             bus,
		cc:              cc,
		matchVolumeIdRe: volumeIDRe,
	}
	c.proxyVolumes = keyed.NewKeyedRefCountWithLogger(c.newProxyVolumeTracker, le)
	return c, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "rpc volume server")
}

// Execute executes the given controller.
func (c *Controller) Execute(ctx context.Context) error {
	c.proxyVolumes.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case volume.BuildBucketAPI:
		return c.resolveLoadProxyVolume(di, d.BuildBucketAPIVolumeID())
	case volume.BuildObjectStoreAPI:
		return c.resolveLoadProxyVolume(di, d.BuildObjectStoreAPIVolumeID())
	case volume.ListBuckets:
		return c.resolveLoadProxyVolumeIDList(di, d.ListBucketsVolumeIDList())
	case bucket.ApplyBucketConfig:
		return c.resolveLoadProxyVolumeIDList(di, d.ApplyBucketConfigVolumeIDList())
	case volume.LookupVolume:
		return c.resolveLoadProxyVolume(di, d.LookupVolumeID())
	}

	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// resolveLoadProxyVolume resolves a directive by loading a proxy volume.
// checks the volume ID, ignores if it doesn't match.
func (c *Controller) resolveLoadProxyVolume(
	di directive.Instance,
	volumeID string,
) ([]directive.Resolver, error) {
	if volumeID == "" || !c.checkVolumeID(volumeID) {
		return nil, nil
	}
	return directive.R(NewLoadProxyVolumeResolver(c, di, volumeID), nil)
}

// resolveLoadProxyVolumeIDList checks the regex and the list of ids.
func (c *Controller) resolveLoadProxyVolumeIDList(
	di directive.Instance,
	volumeIDs []string,
) ([]directive.Resolver, error) {
	var volID string
	if len(volumeIDs) != 0 {
		var matched bool
		for _, volumeID := range volumeIDs {
			if matched = c.checkVolumeID(volumeID); matched {
				volID = volumeID
				break
			}
		}
		if !matched {
			return nil, nil
		}
	}
	return directive.R(NewLoadProxyVolumeResolver(c, di, volID), nil)
}

// checkVolumeID checks if the volume id matches the regex.
func (c *Controller) checkVolumeID(volumeID string) bool {
	if c.matchVolumeIdRe == nil {
		return true
	}
	return c.matchVolumeIdRe.MatchString(volumeID)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

package volume_rpc_client

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/db/bucket"
	volume "github.com/s4wave/spacewave/db/volume"
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
	proxyVolumes *keyed.KeyedRefCount[string, *proxyVolumeTracker]
}

// NewController constructs a new rpc volume client.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	cc *Config,
) (*Controller, error) {
	volumeIDRe, err := cc.ParseVolumeIdRe()
	if err != nil {
		return nil, err
	}
	releaseDelay, err := cc.ParseReleaseDelay()
	if err != nil {
		return nil, err
	}
	c := &Controller{
		le:              le,
		bus:             bus,
		cc:              cc,
		matchVolumeIdRe: volumeIDRe,
	}
	c.proxyVolumes = keyed.NewKeyedRefCount(
		c.newProxyVolumeTracker,
		keyed.WithExitLogger[string, *proxyVolumeTracker](le),
		keyed.WithReleaseDelay[string, *proxyVolumeTracker](releaseDelay),
	)
	// add an initial reference to the volume_id_list
	if cc.GetLoadOnStartup() {
		for _, volumeID := range cc.GetVolumeIdList() {
			if volumeID != "" {
				_, _, _ = c.proxyVolumes.AddKeyRef(volumeID)
			}
		}
	}
	return c, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "rpc volume server")
}

// Execute executes the controller goroutine.
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
	case bucket.BuildBucketAPI:
		return c.resolveLoadProxyVolume(di, d.BuildBucketAPIStoreID())
	case volume.LookupVolume:
		return c.resolveLoadProxyVolume(di, d.LookupVolumeID())
	case volume.BuildObjectStoreAPI:
		return c.resolveLoadProxyVolume(di, d.BuildObjectStoreAPIVolumeID())
	case volume.ListBuckets:
		return c.resolveLoadProxyVolumeIDList(di, d.ListBucketsVolumeIDList())
	case bucket.ApplyBucketConfig:
		return c.resolveLoadProxyVolumeIDList(di, d.ApplyBucketConfigVolumeIDList())
	}

	return nil, nil
}

// LoadProxyVolume loads the proxy volume by ID.
// Returns a function to release the volume reference.
// Returns an error if the volume id does not match this controller.
func (c *Controller) LoadProxyVolume(ctx context.Context, volumeID string) (volume.Controller, func(), error) {
	var matched bool
	if volumeID, matched = c.checkVolumeID(volumeID); !matched {
		return nil, nil, errors.New("volume id does not match this rpc client")
	}

	le := c.le.WithField("volume-id", volumeID)
	le.Debug("adding proxy volume reference")
	ref, tracker, _ := c.proxyVolumes.AddKeyRef(volumeID)
	var relOnce sync.Once
	rel := func() {
		relOnce.Do(func() {
			le.Debug("removed proxy volume reference")
			ref.Release()
		})
	}

	proxyVol, err := tracker.proxyVolCtr.WaitValue(ctx, nil)
	if err != nil {
		rel()
		return nil, nil, err
	}

	return proxyVol, rel, nil
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
	var matched bool
	if volumeID, matched = c.checkVolumeID(volumeID); !matched {
		return nil, nil
	}
	return directive.R(NewLoadProxyVolumeResolver(c, di, volumeID), nil)
}

// resolveLoadProxyVolumeIDList checks the regex
func (c *Controller) resolveLoadProxyVolumeIDList(
	di directive.Instance,
	volumeIDs []string,
) ([]directive.Resolver, error) {
	resolverMap := make(map[string]struct{})
	var resolvers []directive.Resolver
	if len(volumeIDs) != 0 {
		for _, volumeID := range volumeIDs {
			volumeID, matched := c.checkVolumeID(volumeID)
			if matched {
				if _, ok := resolverMap[volumeID]; ok {
					continue
				}
				resolverMap[volumeID] = struct{}{}
				resolvers = append(resolvers, NewLoadProxyVolumeResolver(c, di, volumeID))
			}
		}
	}
	return resolvers, nil
}

// checkVolumeID checks if the volume id matches the regex or list.
// returns the updated volume id if aliased
func (c *Controller) checkVolumeID(volumeID string) (string, bool) {
	if volumeID == "" {
		return volumeID, false
	}
	// if there are no values set in these fields, match any.
	volumeIDList := c.cc.GetVolumeIdList()
	volumeAliases := c.cc.GetVolumeAliases()
	if c.matchVolumeIdRe == nil && len(volumeIDList) == 0 && len(volumeAliases) == 0 {
		return volumeID, true
	}
	for to, alias := range volumeAliases {
		if slices.Contains(alias.GetFrom(), volumeID) {
			return to, true
		}
	}
	if slices.Contains(volumeIDList, volumeID) {
		return volumeID, true
	}
	if c.matchVolumeIdRe == nil {
		return "", false
	}
	return volumeID, c.matchVolumeIdRe.MatchString(volumeID)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

package volume_rpc_server

import (
	"context"
	"regexp"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	volume "github.com/aperturerobotics/hydra/volume"
	volume_rpc "github.com/aperturerobotics/hydra/volume/rpc"
	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the rpc volume server controller id.
const ControllerID = "hydra/volume/rpc/server"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Controller implements the rpc volume server.
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
	// mux is the srpc mux with the AccessVolumes service
	mux srpc.Mux
	// proxyVolumes is the list of proxy volume trackers.
	proxyVolumes *keyed.KeyedRefCount[string, *proxyVolumeTracker]
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
	releaseDelay, err := cc.ParseReleaseDelay()
	if err != nil {
		return nil, err
	}
	mux := srpc.NewMux()
	c := &Controller{
		le:              le,
		cc:              cc,
		bus:             bus,
		mux:             mux,
		matchVolumeIdRe: volumeIDRe,
	}
	if err := mux.Register(volume_rpc.NewSRPCAccessVolumesHandler(c, cc.GetServiceId())); err != nil {
		return nil, err
	}
	c.proxyVolumes = keyed.NewKeyedRefCount(
		c.newProxyVolumeTracker,
		keyed.WithExitLogger[string, *proxyVolumeTracker](le),
		keyed.WithReleaseDelay[string, *proxyVolumeTracker](releaseDelay),
	)
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
	case bifrost_rpc.LookupRpcService:
		serviceID := d.LookupRpcServiceID()
		if c.cc.GetServiceId() == serviceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}

	return nil, nil
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	if serviceID != "" && c.cc.GetServiceId() != serviceID {
		return false, nil
	}
	return c.mux.InvokeMethod("", methodID, strm)
}

// WatchVolumeInfo watches information about a volume.
// The most recent message contains the most recently known state.
// If the volume was not found (directive is idle) returns empty.
func (c *Controller) WatchVolumeInfo(
	req *volume_rpc.WatchVolumeInfoRequest,
	strm volume_rpc.SRPCAccessVolumes_WatchVolumeInfoStream,
) error {
	// check if the volume id matches
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return volume.ErrVolumeIDEmpty
	}
	if !c.checkVolumeID(volumeID) {
		return errors.Wrap(volume_rpc.ErrUnknownVolumeID, volumeID)
	}

	// create the volume tracker
	ref, tracker, _ := c.proxyVolumes.AddKeyRef(volumeID)
	defer ref.Release()

	// watch the volume for changes
	ctx := strm.Context()
	var err error
	var currProxyVol *ProxyVolume
	for {
		currProxyVol, err = tracker.proxyVolCtr.WaitValueChange(ctx, currProxyVol, nil)
		if err != nil {
			return err
		}

		if currProxyVol == nil {
			// became not-found when previously found
			err := strm.Send(&volume_rpc.WatchVolumeInfoResponse{
				NotFound: true,
			})
			if err != nil {
				return err
			}
		} else {
			currVol := currProxyVol.GetVolume()
			volInfo, err := volume.NewVolumeInfo(ctx, nil, currVol)
			if err != nil {
				return err
			}
			err = strm.Send(&volume_rpc.WatchVolumeInfoResponse{
				VolumeInfo: volInfo,
			})
			if err != nil {
				return err
			}
		}
	}
}

// VolumeRpc uses the LookupVolume directive access a Volume handle.
// Exposes the ProxyVolume service.
// Id: volume id
func (c *Controller) VolumeRpc(strm volume_rpc.SRPCAccessVolumes_VolumeRpcStream) error {
	return rpcstream.HandleRpcStream(strm, c.GetRpcStreamMux)
}

// GetRpcStreamMux returns the mux for the given volume id proxy service.
func (c *Controller) GetRpcStreamMux(ctx context.Context, volumeID string) (srpc.Invoker, func(), error) {
	if !c.checkVolumeID(volumeID) {
		return nil, nil, volume_rpc.ErrUnknownVolumeID
	}

	ref, tracker, _ := c.proxyVolumes.AddKeyRef(volumeID)
	mux, err := tracker.waitMux(ctx)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}

	return mux, ref.Release, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// checkVolumeID checks if the volume id matches the regex or list.
func (c *Controller) checkVolumeID(volumeID string) bool {
	if c.matchVolumeIdRe != nil && c.matchVolumeIdRe.MatchString(volumeID) {
		return true
	}
	for _, val := range c.cc.GetVolumeIdList() {
		if val == volumeID {
			return true
		}
	}
	return false
}

// _ is a type assertion
var (
	_ controller.Controller              = ((*Controller)(nil))
	_ srpc.Invoker                       = ((*Controller)(nil))
	_ volume_rpc.SRPCAccessVolumesServer = ((*Controller)(nil))
)

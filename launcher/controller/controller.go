package bldr_launcher_controller

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/bifrost/peer"
	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_launcher "github.com/aperturerobotics/bldr/launcher"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/launcher/controller"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller manages running the launcher.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// mux implements the Launcher RPC service
	mux srpc.Mux

	// endps is the endpoint url list
	endps []*HttpEndpoint
	// distPeerIDs is the list of distribution peer ids
	distPeerIDs []peer.ID
	// launcherInfoCtr contains the current launcher info.
	// the pointer changes when updated (immutable object)
	launcherInfoCtr *ccontainer.CContainer[*bldr_launcher.LauncherInfo]
	// confFetcherRoutine fetches configurations from the list of endpoints.
	// tries each endpoint in order until it finds a valid dist config
	// stops after finding a valid config
	confFetcherRoutine *routine.RoutineContainer
	// configSetRoutine applies the latest config set from the launcher info
	configSetRoutine *routine.RoutineContainer
	// mtx guards below fields
	mtx sync.Mutex
	// confFetcherRefetch is a timer to restart confFetcherRoutine on success
	confFetcherRefetch *time.Timer
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  srpc.NewMux(),

		launcherInfoCtr: ccontainer.NewCContainer[*bldr_launcher.LauncherInfo](nil),
	}
	ctrl.endps = conf.CloneSortEndpoints()        // checked in Validate
	ctrl.distPeerIDs, _ = conf.ParseDistPeerIds() // checked in Validate
	fetcherBackoffConf := conf.GetEndpointsBackoff()
	if fetcherBackoffConf.GetEmpty() {
		fetcherBackoffConf = defaultFetcherBackoffConf()
	}
	fetcherBackoff := fetcherBackoffConf.Construct()
	ctrl.confFetcherRoutine = routine.NewRoutineContainer(
		// log if it exits
		routine.WithExitLogger(le.WithField("routine", "launcher-endpoints")),
		// backoff: retry if fails
		routine.WithBackoff(fetcherBackoff),
		// schedule a retry upon success as well
		routine.WithExitCb(ctrl.confFetcherExited),
	)
	ctrl.confFetcherRoutine.SetRoutine(ctrl.fetchDistConfig)
	ctrl.configSetRoutine = routine.NewRoutineContainer()
	ctrl.configSetRoutine.SetRoutine(ctrl.applyDistConfigSet)
	_ = bldr_launcher.SRPCRegisterLauncher(ctrl.mux, NewLauncherServer(ctrl))
	return ctrl
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"launcher controller",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	c.le.Info("launcher starting")

	// load the built-in app dist config
	defDistConf, _, defDistConfSigner, err := c.conf.ParseInitDistConfig(c.conf.GetProjectId(), c.distPeerIDs)
	if err == nil && defDistConf != nil {
		c.le.Debug("loaded default app dist config")
	}
	if err != nil {
		c.le.WithError(err).Warn("cannot load default dist config: continuing without")
	}

	// load the initial app dist config
	var distConf *bldr_launcher.DistConfig
	distConfDat, err := c.loadDistConf(ctx)
	if err != nil {
		c.le.WithError(err).Warn("cannot load stored dist config")
		distConfDat = nil
		distConf = nil
	}
	if len(distConfDat) != 0 {
		var distConfSigner peer.ID
		distConf, _, distConfSigner, err = c.parseDistConf(distConfDat)
		if err == nil {
			c.le.
				WithField("conf-rev", distConf.GetRev()).
				WithField("conf-signer", distConfSigner.String()).
				Debug("loaded stored app dist config")
		}
	}
	if defDistConf != nil && distConf.GetRev() < defDistConf.GetRev() {
		distConf = defDistConf
		c.le.
			WithField("defconf-rev", defDistConf.GetRev()).
			WithField("defconf-signer", defDistConfSigner.String()).
			Debug("using default dist config")
	}
	if distConf == nil {
		distConf = &bldr_launcher.DistConfig{}
	}

	// set the initial launcher info object
	c.launcherInfoCtr.SetValue(&bldr_launcher.LauncherInfo{
		DistConfig: distConf,
	})

	// start the dist conf update fetcher
	_ = c.confFetcherRoutine.SetContext(ctx, true)
	_ = c.configSetRoutine.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == bldr_launcher.SRPCLauncherServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	}
	return nil, nil
}

// PushDistConf pushes an updated dist configuration signed packedmsg.
//
// Returns the updated config, found packedmsg substring, signer peer, updated, currentRev, and any error.
// If updated=false the current dist config had equal and/or newer rev and/or the given was invalid.
//
// Finds the latest valid signed dist config in the body.
func (c *Controller) PushDistConf(ctx context.Context, body []byte) (*bldr_launcher.DistConfig, string, peer.ID, bool, uint64, error) {
	currLauncherInfo, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, "", "", false, 0, err
	}
	currDistConf := currLauncherInfo.GetDistConfig()
	currRev := currDistConf.GetRev()

	updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, err := bldr_launcher.ParseDistConfigPackedMsg(
		c.le.WithField("endpoint", "PushDistConf"),
		body,
		c.distPeerIDs,
		c.conf.GetProjectId(),
	)
	rev := updatedAppDistConf.GetRev()
	if err != nil || rev == 0 {
		return nil, "", "", false, currRev, err
	}

	// config is valid: check if newer
	if rev <= currRev {
		return updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, false, currRev, nil
	}

	// valid and updated
	if err := c.storeDistConf(ctx, []byte(updatedAppDistConfMsg)); err != nil {
		c.le.WithError(err).Warn("failed to store updated app dist config")
	}
	_, _ = c.swapDistConf(updatedAppDistConf)
	return updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, true, currRev, nil
}

// swapDistConf swaps in a new dist conf to the launcher info if the revision is higher.
//
// Returns the stored config and a bool if updated.
// Does not store the updated config in storage.
func (c *Controller) swapDistConf(updConf *bldr_launcher.DistConfig) (*bldr_launcher.DistConfig, bool) {
	nextVal, changed, _ := c.modifyLauncherInfo(func(info *bldr_launcher.LauncherInfo) (commit bool, cbErr error) {
		if info.GetDistConfig().GetRev() >= updConf.GetRev() {
			return false, nil
		}
		info.DistConfig = updConf
		return true, nil
	})
	return nextVal.GetDistConfig(), changed
}

// modifyLauncherInfo atomically modifies & swaps a new launcher info in if changed.
// does nothing if cb returns an error
// cb should edit the passed object
// if cb returns false, nil, does nothing
func (c *Controller) modifyLauncherInfo(
	cb func(info *bldr_launcher.LauncherInfo) (commit bool, cbErr error),
) (nextVal *bldr_launcher.LauncherInfo, changed bool, rerr error) {
	_ = c.launcherInfoCtr.SwapValue(func(val *bldr_launcher.LauncherInfo) *bldr_launcher.LauncherInfo {
		modifyVal := val.CloneVT()
		if modifyVal == nil {
			modifyVal = &bldr_launcher.LauncherInfo{}
		}
		commit, err := cb(modifyVal)
		if err != nil || !commit || modifyVal.EqualVT(val) {
			rerr = err
			nextVal = val.CloneVT()
			return val
		}
		changed = true
		nextVal = modifyVal
		return modifyVal.CloneVT()
	})
	return
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	c.mtx.Lock()
	if c.confFetcherRefetch != nil {
		_ = c.confFetcherRefetch.Stop()
		c.confFetcherRefetch = nil
	}
	c.mtx.Unlock()
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

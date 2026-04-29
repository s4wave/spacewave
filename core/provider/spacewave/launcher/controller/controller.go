package spacewave_launcher_controller

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver/v4"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "spacewave/launcher/controller"

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
	launcherInfoCtr *ccontainer.CContainer[*spacewave_launcher.LauncherInfo]
	// fetchStatusCtr publishes DistConfig fetch-status snapshots. Consumers
	// (e.g. spacewave-loader) observe transitions via the
	// WatchLauncherFetchStatus directive to drive loading-UI retry messages.
	fetchStatusCtr *ccontainer.CContainer[*spacewave_launcher.FetchStatus]
	// confFetcherRoutine fetches configurations from the list of endpoints.
	// tries each endpoint in order until it finds a valid dist config
	// stops after finding a valid config
	confFetcherRoutine *routine.RoutineContainer
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
	distPeerIDs []peer.ID,
	endpoints []*HttpEndpoint,
) *Controller {
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  srpc.NewMux(),

		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](nil),
		fetchStatusCtr: ccontainer.NewCContainer[*spacewave_launcher.FetchStatus](
			&spacewave_launcher.FetchStatus{},
		),
	}
	ctrl.endps = endpoints
	ctrl.distPeerIDs = distPeerIDs
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
	_ = spacewave_launcher.SRPCRegisterLauncher(ctrl.mux, NewLauncherServer(ctrl))
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
	var distConf *spacewave_launcher.DistConfig
	loadedPackageDistConf := false
	distConfDat, err := c.loadDistConf(ctx)
	if err != nil {
		c.le.WithError(err).Warn("cannot load stored dist config")
		distConfDat = nil
		distConf = nil
	}
	if len(distConfDat) == 0 {
		localDistConfDat, localDistConfPath, localErr := c.loadLocalDistConf()
		if localErr != nil {
			c.le.WithError(localErr).Warn("cannot load package dist config")
		}
		if len(localDistConfDat) != 0 {
			distConfDat = localDistConfDat
			loadedPackageDistConf = true
			c.le.WithField("path", localDistConfPath).Info("loaded package dist config")
		}
	}
	if len(distConfDat) != 0 {
		var distConfSigner peer.ID
		distConf, _, distConfSigner, err = c.parseDistConf(distConfDat)
		if err == nil {
			c.le.
				WithField("conf-rev", distConf.GetRev()).
				WithField("conf-signer", distConfSigner.String()).
				Debug("loaded app dist config")
			if loadedPackageDistConf {
				storeErr := c.storeDistConf(ctx, distConfDat)
				if storeErr == nil {
					c.le.Info("persisted package dist config to storage")
				}
				if storeErr != nil {
					c.le.WithError(storeErr).Warn("cannot persist dist config")
				}
			}
		}
	}
	distConfRev := uint64(0)
	if distConf != nil {
		distConfRev = distConf.GetRev()
	}
	if defDistConf != nil && distConfRev < defDistConf.GetRev() {
		distConf = defDistConf
		c.le.
			WithField("defconf-rev", defDistConf.GetRev()).
			WithField("defconf-signer", defDistConfSigner.String()).
			Debug("using default dist config")
	}
	if distConf == nil {
		distConf = &spacewave_launcher.DistConfig{}
	}

	// set the initial launcher info object
	c.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig: distConf,
	})
	// seed fetch status with whether a non-empty dist config was found on disk
	// or in the embedded default so downstream watchers start in the right
	// state (has-config -> skip retry UI; no-config -> show connecting).
	c.fetchStatusCtr.SetValue(&spacewave_launcher.FetchStatus{
		HasConfig: distConf.GetRev() != 0,
	})

	// start the dist conf update fetcher
	_ = c.confFetcherRoutine.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == spacewave_launcher.SRPCLauncherServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	case spacewave_launcher.RecheckDistConfig:
		projectID := d.RecheckDistConfigProjectID()
		if projectID != "" && projectID != c.conf.GetProjectId() {
			return nil, nil
		}
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			c.RecheckDistConfig()
			_, accepted := handler.AddValue(spacewave_launcher.RecheckDistConfigValue(true))
			if !accepted {
				return nil
			}
			handler.MarkIdle(true)
			<-ctx.Done()
			return nil
		}), nil)
	case spacewave_launcher.WatchLauncherFetchStatus:
		projectID := d.WatchLauncherFetchStatusProjectID()
		if projectID != "" && projectID != c.conf.GetProjectId() {
			return nil, nil
		}
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			var curr *spacewave_launcher.FetchStatus
			var currVid uint32
			for {
				next, err := c.fetchStatusCtr.WaitValueChange(ctx, curr, nil)
				if err != nil {
					return err
				}
				if next == curr {
					continue
				}
				if currVid != 0 {
					handler.RemoveValue(currVid)
					currVid = 0
				}
				curr = next
				if curr == nil {
					continue
				}
				vid, accepted := handler.AddValue(spacewave_launcher.WatchLauncherFetchStatusValue(curr))
				if !accepted {
					curr = nil
					continue
				}
				currVid = vid
				handler.MarkIdle(true)
			}
		}), nil)
	}
	return nil, nil
}

// PushDistConf pushes an updated dist configuration signed packedmsg.
//
// Returns the updated config, found packedmsg substring, signer peer, updated, currentRev, and any error.
// If updated=false the current dist config had equal and/or newer rev and/or the given was invalid.
//
// Finds the latest valid signed dist config in the body.
func (c *Controller) PushDistConf(ctx context.Context, body []byte) (*spacewave_launcher.DistConfig, string, peer.ID, bool, uint64, error) {
	currLauncherInfo, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, "", "", false, 0, err
	}
	currDistConf := currLauncherInfo.GetDistConfig()
	currRev := currDistConf.GetRev()

	updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, err := spacewave_launcher.ParseDistConfigPackedMsg(
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
func (c *Controller) swapDistConf(updConf *spacewave_launcher.DistConfig) (*spacewave_launcher.DistConfig, bool) {
	nextVal, changed, _ := c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (commit bool, cbErr error) {
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
	cb func(info *spacewave_launcher.LauncherInfo) (commit bool, cbErr error),
) (nextVal *spacewave_launcher.LauncherInfo, changed bool, rerr error) {
	_ = c.launcherInfoCtr.SwapValue(func(val *spacewave_launcher.LauncherInfo) *spacewave_launcher.LauncherInfo {
		modifyVal := val.CloneVT()
		if modifyVal == nil {
			modifyVal = &spacewave_launcher.LauncherInfo{}
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

// RecheckDistConfig triggers an immediate re-fetch of the app dist config.
// Cancels any pending refetch timer and restarts the fetcher routine.
func (c *Controller) RecheckDistConfig() {
	c.mtx.Lock()
	if c.confFetcherRefetch != nil {
		_ = c.confFetcherRefetch.Stop()
		c.confFetcherRefetch = nil
	}
	_ = c.confFetcherRoutine.RestartRoutine()
	c.mtx.Unlock()
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

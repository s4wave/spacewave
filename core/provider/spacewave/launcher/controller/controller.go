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
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "spacewave/launcher/controller"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

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
	// launcherInfoCtr contains the current launcher info, including the
	// DistConfig fetch status. Values are immutable: every change swaps in a
	// new pointer.
	launcherInfoCtr *ccontainer.CContainer[*spacewave_launcher.LauncherInfo]
	// confFetcherRoutine fetches configurations from the list of endpoints.
	// tries each endpoint in order until it finds a valid dist config
	// stops after finding a valid config
	confFetcherRoutine *routine.RoutineContainer
	// releaseMetadataRoutine resolves the current DistConfig against the
	// Release World and stages the native entrypoint update when one applies.
	releaseMetadataRoutine *routine.RoutineContainer
	// configSetRoutine applies signed launcher controller configs carried by
	// the current DistConfig.
	configSetRoutine *routine.RoutineContainer
	// stagingDirFunc overrides the platform staging dir in tests
	stagingDirFunc func() (string, error)
	// currentExecutableBundleFunc overrides current executable bundle detection in tests.
	currentExecutableBundleFunc func() (execPath string, isBundle bool, bundleRoot string, err error)
	// adoptMtx orders adoptDistConf calls.
	adoptMtx sync.Mutex
	// mtx guards below fields
	mtx sync.Mutex
	// confFetcherRefetch is a timer to restart confFetcherRoutine on success
	confFetcherRefetch *time.Timer
	// daemonUpdateWatchers counts live serving-daemon update streams.
	daemonUpdateWatchers int
	// daemonUpdateClaimed suppresses watch-loss failure after the idle claim.
	daemonUpdateClaimed bool
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	distPeerIDs []peer.ID,
	endpoints []*HttpEndpoint,
) *Controller {
	// Initialize the launcher and its immutable resolved distribution pins.
	ctrl := &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
		mux:  srpc.NewMux(),

		launcherInfoCtr: ccontainer.NewCContainer[*spacewave_launcher.LauncherInfo](nil),
	}
	ctrl.endps = endpoints
	ctrl.distPeerIDs = distPeerIDs

	// Configure the endpoint fetch lifetime and backoff.
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

	// Resolve and stage metadata on the launcher release routine.
	ctrl.releaseMetadataRoutine = routine.NewRoutineContainer(
		routine.WithExitLogger(le.WithField("routine", "launcher-release-metadata")),
		routine.WithBackoff(fetcherBackoffConf.Construct()),
	)
	ctrl.releaseMetadataRoutine.SetRoutine(ctrl.refreshCurrentReleaseMetadataStatus)

	// Apply accepted distribution configs and expose launcher RPC services.
	ctrl.configSetRoutine = routine.NewRoutineContainer(
		routine.WithExitLogger(le.WithField("routine", "launcher-config-set")),
	)
	ctrl.configSetRoutine.SetRoutine(ctrl.applyDistConfigSet)
	_ = spacewave_launcher.SRPCRegisterLauncher(ctrl.mux, NewLauncherServer(ctrl))
	_ = manifest.SRPCRegisterReleaseAuthority(ctrl.mux, NewReleaseAuthorityServer(ctrl))
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
	// Announce launcher startup before reading persisted configuration.
	c.le.Info("launcher starting")

	// load the built-in app dist config
	defDistConf, defDistConfMsg, defDistConfSigner, err := c.conf.ParseInitDistConfig(c.conf.GetProjectId(), c.distPeerIDs)
	if err == nil && defDistConf != nil {
		c.le.Debug("loaded default app dist config")
	}
	if err != nil {
		c.le.WithError(err).Warn("cannot load default dist config: continuing without")
	}

	// load the initial app dist config
	var distConf *spacewave_launcher.DistConfig
	var distConfMsg string
	distConfSource := spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_NONE
	loadedPackageDistConf := false
	distConfDat, err := c.loadDistConf(ctx)
	if err != nil {
		c.le.WithError(err).Warn("cannot load stored dist config")
		distConfDat = nil
		distConf = nil
	}
	if len(distConfDat) != 0 {
		distConfSource = spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_STORED
	}
	if len(distConfDat) == 0 {
		localDistConfDat, localDistConfPath, localErr := c.loadLocalDistConf()
		if localErr != nil {
			c.le.WithError(localErr).Warn("cannot load package dist config")
		}
		if len(localDistConfDat) != 0 {
			distConfDat = localDistConfDat
			loadedPackageDistConf = true
			distConfSource = spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_PACKAGE
			c.le.WithField("path", localDistConfPath).Info("loaded package dist config")
		}
	}

	// Authenticate stored or package-provided distribution configuration.
	if len(distConfDat) != 0 {
		var distConfSigner peer.ID
		distConf, distConfMsg, distConfSigner, err = c.parseDistConf(distConfDat)
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

	// Select the newest valid persisted or embedded configuration.
	distConfRev := uint64(0)
	if distConf != nil {
		distConfRev = distConf.GetRev()
	}
	if defDistConf != nil && distConfRev < defDistConf.GetRev() {
		distConf = defDistConf
		distConfMsg = defDistConfMsg
		distConfSource = spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_EMBEDDED_DEFAULT
		c.le.
			WithField("defconf-rev", defDistConf.GetRev()).
			WithField("defconf-signer", defDistConfSigner.String()).
			Debug("using default dist config")
	}
	if distConf == nil {
		distConf = &spacewave_launcher.DistConfig{}
	}

	// Publish the initial launcher info. The fetch status records whether a
	// DistConfig was found so watchers start in the right state: with a
	// config the loader skips its retry UI, without one it shows connecting.
	c.launcherInfoCtr.SetValue(&spacewave_launcher.LauncherInfo{
		DistConfig:    distConf,
		DistConfigMsg: distConfMsg,
		FetchStatus: &spacewave_launcher.FetchStatus{
			HasConfig:              distConf.GetRev() != 0,
			SelectedConfigRev:      distConf.GetRev(),
			SelectedConfigSource:   distConfSource,
			ReleaseMetadataOutcome: spacewave_launcher.ReleaseMetadataOutcome_RELEASE_METADATA_OUTCOME_PENDING,
		},
	})
	_ = c.configSetRoutine.SetContext(ctx, true)
	_ = c.releaseMetadataRoutine.SetContext(ctx, true)

	// start the dist conf update fetcher
	if len(c.endps) != 0 {
		_ = c.confFetcherRoutine.SetContext(ctx, true)
	}
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == manifest.SRPCReleaseAuthorityServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
		if d.LookupRpcServiceID() == spacewave_launcher.SRPCLauncherServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	case spacewave_launcher.RecheckDistConfig:
		projectID := d.RecheckDistConfigProjectID()
		if projectID != "" && projectID != c.conf.GetProjectId() {
			return nil, nil
		}
		return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
			// Recheck the selected project and acknowledge the request.
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
		return directive.R(directive.NewFuncResolver(c.resolveWatchFetchStatus), nil)
	}
	return nil, nil
}

// resolveWatchFetchStatus replaces the WatchLauncherFetchStatus value whenever
// the fetch status in the launcher info changes.
func (c *Controller) resolveWatchFetchStatus(ctx context.Context, handler directive.ResolverHandler) error {
	var info *spacewave_launcher.LauncherInfo
	var curr *spacewave_launcher.FetchStatus
	var currVid uint32
	for {
		// Wait for launcher info whose fetch status differs from the value.
		var err error
		info, err = c.launcherInfoCtr.WaitValueChange(ctx, info, nil)
		if err != nil {
			return err
		}
		next := info.GetFetchStatus()
		if next == nil || next.EqualVT(curr) {
			continue
		}

		// Replace the previous value with the new status.
		if currVid != 0 {
			handler.RemoveValue(currVid)
			currVid = 0
		}
		curr = next
		vid, accepted := handler.AddValue(curr)
		if !accepted {
			curr = nil
			continue
		}
		currVid = vid
		handler.MarkIdle(true)
	}
}

// updateFetchStatus applies update to a copy of the fetch status and publishes
// the launcher info when it changed.
func (c *Controller) updateFetchStatus(update func(*spacewave_launcher.FetchStatus)) {
	_, _, _ = c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		if info.FetchStatus == nil {
			info.FetchStatus = &spacewave_launcher.FetchStatus{}
		}
		update(info.FetchStatus)
		return true, nil
	})
}

// PushDistConf adopts the newest valid signed DistConfig found in body when
// its revision is higher than the current one.
//
// Returns the found config, whether it was adopted, and the previous
// revision. An invalid body, including a config for another channel, returns
// an error.
func (c *Controller) PushDistConf(ctx context.Context, body []byte) (*spacewave_launcher.DistConfig, bool, uint64, error) {
	// Read the current revision before authenticating a pushed configuration.
	currLauncherInfo, err := c.launcherInfoCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, false, 0, err
	}
	currRev := currLauncherInfo.GetDistConfig().GetRev()

	// Verify the signature, project and channel as for an endpoint response.
	distConf, distConfMsg, distConfSigner, err := c.parseDistConf(body)
	if err != nil {
		return nil, false, currRev, err
	}

	// Adopt the config if it is newer.
	if !c.adoptDistConf(ctx, distConf, distConfMsg, spacewave_launcher.DistConfigSource_DIST_CONFIG_SOURCE_PUSH) {
		return distConf, false, currRev, nil
	}
	c.le.
		WithField("prev-conf-rev", currRev).
		WithField("conf-rev", distConf.GetRev()).
		WithField("conf-signer", distConfSigner.String()).
		Info("adopted pushed app dist config")
	return distConf, true, currRev, nil
}

// adoptDistConf selects conf, parsed from the signed msg, when its revision
// is higher than the current one. It then persists msg, records source in the
// fetch status and rechecks the release metadata. adoptMtx orders the swap
// and the store so storage never regresses to an older revision.
//
// Returns true if conf was adopted.
func (c *Controller) adoptDistConf(
	ctx context.Context,
	conf *spacewave_launcher.DistConfig,
	msg string,
	source spacewave_launcher.DistConfigSource,
) bool {
	// Serialize adoption and persistence across concurrent configuration updates.
	c.adoptMtx.Lock()
	defer c.adoptMtx.Unlock()

	// Select the config only if it is newer.
	_, adopted, _ := c.modifyLauncherInfo(func(info *spacewave_launcher.LauncherInfo) (bool, error) {
		// Publish only a strictly newer accepted configuration.
		if info.GetDistConfig().GetRev() >= conf.GetRev() {
			return false, nil
		}
		info.DistConfig = conf
		info.DistConfigMsg = msg
		if info.FetchStatus == nil {
			info.FetchStatus = &spacewave_launcher.FetchStatus{}
		}
		info.FetchStatus.HasConfig = true
		info.FetchStatus.SelectedConfigRev = conf.GetRev()
		info.FetchStatus.SelectedConfigSource = source
		return true, nil
	})
	if !adopted {
		return false
	}

	// Persist the config and resolve its release.
	if err := c.storeDistConf(ctx, []byte(msg)); err != nil {
		c.le.WithError(err).Warn("failed to store updated app dist config")
	}
	c.RecheckReleaseMetadata()
	return true
}

// modifyLauncherInfo atomically modifies & swaps a new launcher info in if changed.
// does nothing if cb returns an error
// cb should edit the passed object
// if cb returns false, nil, does nothing
func (c *Controller) modifyLauncherInfo(
	cb func(info *spacewave_launcher.LauncherInfo) (commit bool, cbErr error),
) (nextVal *spacewave_launcher.LauncherInfo, changed bool, rerr error) {
	_ = c.launcherInfoCtr.SwapValue(func(val *spacewave_launcher.LauncherInfo) *spacewave_launcher.LauncherInfo {
		// Edit a private copy and keep the old value on rejection or no change.
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

// RecheckReleaseMetadata restarts release metadata resolution for the current DistConfig.
func (c *Controller) RecheckReleaseMetadata() {
	if c.releaseMetadataRoutine == nil {
		return
	}
	_ = c.releaseMetadataRoutine.RestartRoutine()
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// Stop pending refetch work under the launcher lock.
	c.mtx.Lock()
	if c.confFetcherRefetch != nil {
		_ = c.confFetcherRefetch.Stop()
		c.confFetcherRefetch = nil
	}
	c.mtx.Unlock()
	if c.releaseMetadataRoutine != nil {
		c.releaseMetadataRoutine.ClearContext()
	}
	if c.configSetRoutine != nil {
		c.configSetRoutine.ClearContext()
	}
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)

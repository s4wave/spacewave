//go:build !js

package bldr_project_controller

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = ConfigID

// Controller is the bldr Project controller.
type Controller struct {
	// le is the root logger.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus

	// manifestBuilders retains builds by their base58-encoded configuration.
	manifestBuilders *keyed.KeyedRefCount[string, *manifestBuilderTracker]
	// remotes is the set of keyed remote access controllers.
	remotes *keyed.KeyedRefCount[string, *remoteTracker]
	// startup manages the set of "start" plugins listed in the config.
	startup *routine.StateRoutineContainer[*bldr_project.StartConfig]

	// statusSinkMtx guards status sinks and build target metadata.
	statusSinkMtx sync.Mutex
	// manifestBuilderStatusSink receives manifest build status events.
	manifestBuilderStatusSink ManifestBuilderStatusSink
	// projectConfigStatusSink receives project config status events.
	projectConfigStatusSink ProjectConfigStatusSink
	// manifestBuilderBuildTargets records finite build targets by builder key.
	manifestBuilderBuildTargets map[string][]string

	// mtx serializes project config changes and builder registration.
	mtx sync.Mutex
	// conf is the current controller config.
	conf atomic.Pointer[Config]
	// routines owns the post-Execute tracker and startup lifetimes.
	routines web_pkg.RoutineGroup
	// lifecycleMtx guards close state and closeDone.
	lifecycleMtx sync.Mutex
	// closed rejects work after shutdown begins.
	closed bool
	// closeDone closes after all owned routines have stopped.
	closeDone chan struct{}
}

// errControllerClosed is returned when the controller is already closed.
var errControllerClosed = errors.New("bldr project controller is closed")

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	// Registries are initialized here and started by Execute.
	ctrl := &Controller{
		le:  le,
		bus: bus,
	}
	ctrl.conf.Store(cc)
	buildBackoff := cc.GetBuildBackoff()
	ctrl.manifestBuilders = keyed.NewKeyedRefCountWithLogger(
		func(key string) (keyed.Routine, *manifestBuilderTracker) {
			r, tracker := ctrl.newManifestBuilderTracker(key)
			return ctrl.routines.Wrap(r), tracker
		},
		le,
		keyed.WithRetry[string, *manifestBuilderTracker](buildBackoff),
	)
	ctrl.remotes = keyed.NewKeyedRefCountWithLogger(
		func(key string) (keyed.Routine, *remoteTracker) {
			r, tracker := ctrl.newRemoteTracker(key)
			return ctrl.routines.Wrap(r), tracker
		},
		le,
		keyed.WithRetry[string, *remoteTracker](buildBackoff),
	)
	ctrl.manifestBuilderBuildTargets = make(map[string][]string)

	// Startup tasks share the controller's shutdown group with build routines.
	ctrl.startup = routine.NewStateRoutineContainerWithLoggerVT[*bldr_project.StartConfig](le, routine.WithRetry(buildBackoff))
	ctrl.startup.SetStateRoutine(func(ctx context.Context, conf *bldr_project.StartConfig) error {
		if !ctrl.routines.Begin() {
			return context.Canceled
		}
		defer ctrl.routines.Done()
		return ctrl.executeStartup(ctx, conf)
	})
	return ctrl
}

// GetConfig returns the current config.
func (c *Controller) GetConfig() *Config {
	return c.conf.Load()
}

// SetManifestBuilderStatusSink sets the manifest builder status sink.
func (c *Controller) SetManifestBuilderStatusSink(sink ManifestBuilderStatusSink) {
	// Replace the observer before publishing the current build states.
	c.statusSinkMtx.Lock()
	c.manifestBuilderStatusSink = sink
	c.statusSinkMtx.Unlock()

	// Call observers outside the sink lock so they can replace themselves.
	for _, builder := range c.getRunningManifestBuilders() {
		builder.tracker.publishManifestBuilderStatus()
	}
}

// SetProjectConfigStatusSink sets the project config status sink.
func (c *Controller) SetProjectConfigStatusSink(sink ProjectConfigStatusSink) {
	// Publish the current configuration after installing the observer.
	c.statusSinkMtx.Lock()
	c.projectConfigStatusSink = sink
	c.statusSinkMtx.Unlock()

	c.publishProjectConfigStatus(c.conf.Load().GetProjectConfig())
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr project controller",
	)
}

// UpdateProjectConfig applies an updated project config restarting affected manifest builders.
func (c *Controller) UpdateProjectConfig(nextConf *bldr_project.ProjectConfig) error {
	// Reject invalid replacements before changing any running build.
	if err := nextConf.Validate(); err != nil {
		return err
	}

	// Configuration updates and shutdown use lifecycleMtx before mtx.
	c.lifecycleMtx.Lock()
	if c.closed {
		c.lifecycleMtx.Unlock()
		return errControllerClosed
	}
	defer c.lifecycleMtx.Unlock()

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Reconcile startup plugins with the replacement configuration.
	c.startup.SetState(nextConf.GetStart())

	prevCtrlConf := c.conf.Load()
	prevConf := prevCtrlConf.GetProjectConfig()
	if nextConf.EqualVT(prevConf) {
		return nil
	}

	// Store a detached configuration before reconciling its builders.
	nextCtrlConf := prevCtrlConf.CloneVT()
	nextCtrlConf.ProjectConfig = nextConf.CloneVT()
	c.conf.Store(nextCtrlConf)
	c.publishProjectConfigStatus(nextConf)

	// Snapshot the keyed registry for this reconciliation.
	manifestBuilders := c.getRunningManifestBuilders()

	// Track builders retained or restarted by the new manifest set.
	seenManifestBuilders := make(map[string]struct{}, len(manifestBuilders))
	restartedManifestBuilders := make(map[string]struct{}, len(manifestBuilders))

	// Restart builds whose manifest or remote configuration changed.
	nextManifests := nextConf.GetManifests()
	nextRemotes := nextConf.GetRemotes()
	for manifestID, nextManifest := range nextManifests {
		for _, builder := range manifestBuilders {
			// Only this manifest's builders participate in its reconciliation.
			if builder.conf.GetManifestId() != manifestID {
				continue
			}

			// Each keyed routine needs at most one restart per update.
			if _, ok := restartedManifestBuilders[builder.key]; ok {
				continue
			}

			// Builders without a remote are removed after reconciliation.
			remoteConf, remoteConfOk := nextRemotes[builder.conf.RemoteId]
			if !remoteConfOk || remoteConf == nil {
				continue
			}

			// Include unresolved configurations so the build reads current inputs.
			_, wasReset := c.manifestBuilders.RestartRoutine(
				builder.key,
				func(_ string, trk *manifestBuilderTracker) bool {
					// An unloaded manifest must also pick up the replacement.
					if !trk.manifestConf.Load().EqualVT(nextManifest) {
						return true
					}

					currRemoteConf := trk.remoteConf.Load()
					if currRemoteConf == nil {
						// An unresolved remote must read the replacement configuration.
						return true
					}

					if !remoteConf.EqualVT(currRemoteConf) {
						return true
					}

					return false
				},
			)
			if wasReset {
				restartedManifestBuilders[builder.key] = struct{}{}
			}

			// Retain this build even when its inputs did not require a restart.
			seenManifestBuilders[builder.key] = struct{}{}
		}
	}

	// Fail and remove builds whose manifest or remote no longer exists.
	for _, builder := range manifestBuilders {
		if _, ok := seenManifestBuilders[builder.key]; !ok {
			if _, manifestExists := nextManifests[builder.conf.ManifestId]; !manifestExists {
				builder.tracker.failWithError(bldr_project.ErrManifestConfNotFound)
			} else {
				builder.tracker.failWithError(bldr_project.ErrRemoteNotFound)
			}
			c.manifestBuilders.RemoveKey(builder.key)
		}
	}

	return nil
}

// publishProjectConfigStatus publishes a clone of the project config to
// the status sink.
func (c *Controller) publishProjectConfigStatus(projectConfig *bldr_project.ProjectConfig) {
	// Snapshot the observer, then pass it a detached configuration.
	c.statusSinkMtx.Lock()
	sink := c.projectConfigStatusSink
	c.statusSinkMtx.Unlock()
	if sink != nil {
		sink.SetProjectConfigStatus(projectConfig.CloneVT())
	}
}

// BuildManifestBuilderConfigs compiles a set of manifests linking them to the remote object key.
//
// Returns the list of created manifest refs and corresponding object keys.
func (c *Controller) BuildManifestBuilderConfigs(
	ctx context.Context,
	manifestBuilderConfigs []*ManifestBuilderConfig,
) ([]*bldr_manifest.ManifestRef, []string, error) {
	// Register the complete build before a compiler can fetch a dependency.
	refs, err := c.addManifestBuilderRefs(manifestBuilderConfigs, false)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		for _, ref := range refs {
			ref.Release()
		}
	}()

	// Await each retained build and preserve completed results on failure.
	var manifestObjKeys []string
	var manifestRefs []*bldr_manifest.ManifestRef
	for _, ref := range refs {
		result, err := ref.GetResultPromiseContainer().Await(ctx)
		if err != nil {
			return manifestRefs, manifestObjKeys, err
		}

		manifestObjKeys = append(manifestObjKeys, result.GetBuilderConfig().GetObjectKey())
		manifestRefs = append(manifestRefs, result.GetBuilderResult().GetManifestRef())
	}

	return manifestRefs, manifestObjKeys, nil
}

// AddManifestBuilderRef adds a reference to a manifest compiler.
func (c *Controller) AddManifestBuilderRef(conf *ManifestBuilderConfig) (*ManifestBuilderRef, error) {
	refs, err := c.addManifestBuilderRefs([]*ManifestBuilderConfig{conf}, false)
	if err != nil {
		return nil, err
	}
	return refs[0], nil
}

// AddRemoteRef adds a reference to a Remote.
// Returns ErrRemoteNotFound if the remote was not found.
func (c *Controller) AddRemoteRef(remoteID string) (*RemoteRef, error) {
	// Retention and shutdown share the lifecycle-before-registry lock order.
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()

	if c.closed {
		return nil, errControllerClosed
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Keep only remotes declared by the current project configuration.
	projConf := c.conf.Load().GetProjectConfig()
	_, ok := projConf.GetRemotes()[remoteID]
	if !ok {
		return nil, bldr_project.ErrRemoteNotFound
	}

	ref, tracker, _ := c.remotes.AddKeyRef(remoteID)
	return newRemoteRef(ref, tracker), nil
}

// WaitRemote adds a reference to a remote and waits for it to be ready.
func (c *Controller) WaitRemote(ctx context.Context, remoteID string) (world.Engine, *RemoteRef, error) {
	// Retain the remote while its world engine becomes available.
	remoteRef, err := c.AddRemoteRef(remoteID)
	if err != nil {
		return nil, nil, err
	}

	remoteEngPtr, err := remoteRef.GetResultPromise().Await(ctx)
	if err != nil {
		remoteRef.Release()
		return nil, nil, err
	}
	remoteEng := *remoteEngPtr
	return remoteEng, remoteRef, nil
}

// AddFetchManifestBuilderRef retains an active dependency build or starts its
// default configuration when none exists. The caller releases both references.
func (c *Controller) AddFetchManifestBuilderRef(ctx context.Context, manifestMeta *bldr_manifest.ManifestMeta) (*ManifestBuilderRef, *RemoteRef, error) {
	// All dependency builds use the project's configured fetch remote.
	manifestRemoteID := c.conf.Load().GetFetchManifestRemote()
	if manifestRemoteID == "" {
		return nil, nil, errors.Wrap(bldr_project.ErrEmptyRemoteID, "fetch_manifest: in project controller config")
	}

	_, remoteRef, err := c.WaitRemote(ctx, manifestRemoteID)
	if err != nil {
		return nil, nil, err
	}

	baseObjKey := remoteRef.tracker.remote.GetObjectKey()
	if baseObjKey == "" {
		remoteRef.Release()
		return nil, nil, errors.Wrap(world.ErrEmptyObjectKey, "fetch_manifest: remote")
	}

	// An unspecified build type retains the development-build default.
	buildType := manifestMeta.GetBuildType()
	if buildType == "" {
		buildType = string(bldr_manifest.BuildType_DEV)
		manifestMeta.BuildType = buildType
	}

	// Package providers must finish before the consuming compiler starts.
	projectConfig := c.conf.Load().GetProjectConfig()
	webPkgDeps := resolveWebPkgDeps(c.le, projectConfig.GetManifests())
	dependencyRefs := make([]*ManifestBuilderRef, 0, len(webPkgDeps[manifestMeta.GetManifestId()]))
	defer func() {
		for _, dependencyRef := range dependencyRefs {
			dependencyRef.Release()
		}
	}()
	for _, dependencyID := range webPkgDeps[manifestMeta.GetManifestId()] {
		dependencyRef, err := c.addFetchManifestBuildRef(NewManifestBuilderConfig(
			dependencyID,
			buildType,
			manifestMeta.GetPlatformId(),
			manifestRemoteID,
		))
		if err != nil {
			remoteRef.Release()
			return nil, nil, err
		}
		dependencyRefs = append(dependencyRefs, dependencyRef)
	}
	for i, dependencyID := range webPkgDeps[manifestMeta.GetManifestId()] {
		if _, err := dependencyRefs[i].GetResultPromiseContainer().Await(ctx); err != nil {
			remoteRef.Release()
			return nil, nil, errors.Wrapf(err, "build fetch manifest dependency %q", dependencyID)
		}
	}

	// Reuse the configured build selected by the enclosing target.
	manifestBuilderRef, err := c.addFetchManifestBuildRef(NewManifestBuilderConfig(
		manifestMeta.GetManifestId(),
		buildType,
		manifestMeta.GetPlatformId(),
		manifestRemoteID,
	))
	if err != nil {
		remoteRef.Release()
		return nil, nil, err
	}
	return manifestBuilderRef, remoteRef, nil
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Start owned registries only while the controller still accepts work.
	c.lifecycleMtx.Lock()
	if c.closed {
		c.lifecycleMtx.Unlock()
		return context.Canceled
	}

	// Keyed routines retain ctx after Execute returns.
	c.manifestBuilders.SetContext(ctx, true)
	c.remotes.SetContext(ctx, true)
	start := c.GetConfig().GetStart()
	c.lifecycleMtx.Unlock()

	if start {
		c.StartStartup(ctx)
	}

	return nil
}

// StartStartup loads the plugins in the project start config while ctx is active.
func (c *Controller) StartStartup(ctx context.Context) {
	// Startup belongs to the same lifetime as the build and remote registries.
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()
	if c.closed {
		return
	}
	c.startup.SetContext(ctx, true)
	c.startup.SetState(c.GetConfig().GetProjectConfig().GetStart())
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	// Closed controllers cannot contribute new dependency resolvers.
	c.lifecycleMtx.Lock()
	closed := c.closed
	c.lifecycleMtx.Unlock()
	if closed {
		return nil, nil
	}

	// Project manifests are available through the ordinary fetch directive.
	dir := di.GetDirective()
	switch d := dir.(type) {
	case bldr_manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(di, d), nil)
	}

	return nil, nil
}

// Close cancels owned work and joins its routines before returning.
func (c *Controller) Close() error {
	// Concurrent callers join the same shutdown operation.
	c.lifecycleMtx.Lock()
	if c.closed {
		done := c.closeDone
		c.lifecycleMtx.Unlock()
		<-done
		return nil
	}
	c.closed = true
	c.closeDone = make(chan struct{})
	done := c.closeDone
	c.routines.StopAccepting()
	c.lifecycleMtx.Unlock()

	// Cancel the registries before waiting for all accepted work to finish.
	c.manifestBuilders.ClearContext()
	c.remotes.ClearContext()
	c.startup.ClearContext()
	c.routines.Wait()
	close(done)
	return nil
}

// _ verifies the controller interface.
var _ controller.Controller = (*Controller)(nil)

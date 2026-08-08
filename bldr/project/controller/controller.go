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
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus

	// manifestBuilders is the set of keyed build controllers.
	// NOTE: this will eventually be replaced with Forge jobs.
	// key is the ManifestBuilderConfig object in b58 format.
	manifestBuilders *keyed.KeyedRefCount[string, *manifestBuilderTracker]
	// remotes is the set of keyed remote access controllers.
	remotes *keyed.KeyedRefCount[string, *remoteTracker]
	// startup manages the set of "start" plugins listed in the config.
	startup *routine.StateRoutineContainer[*bldr_project.StartConfig]
	// statusSinkMtx guards status sinks and build target metadata
	statusSinkMtx sync.Mutex
	// manifestBuilderStatusSink receives manifest build status events
	manifestBuilderStatusSink ManifestBuilderStatusSink
	// projectConfigStatusSink receives project config status events
	projectConfigStatusSink ProjectConfigStatusSink
	// manifestBuilderBuildTargets records finite build targets by manifest builder key
	manifestBuilderBuildTargets map[string][]string
	// mtx guards writing below fields
	mtx sync.Mutex
	// conf is the current controller config
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

var errControllerClosed = errors.New("bldr project controller is closed")

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
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
	c.statusSinkMtx.Lock()
	c.manifestBuilderStatusSink = sink
	c.statusSinkMtx.Unlock()

	for _, builder := range c.getRunningManifestBuilders() {
		builder.tracker.publishManifestBuilderStatus()
	}
}

// SetProjectConfigStatusSink sets the project config status sink.
func (c *Controller) SetProjectConfigStatusSink(sink ProjectConfigStatusSink) {
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
	if err := nextConf.Validate(); err != nil {
		return err
	}

	c.lifecycleMtx.Lock()
	if c.closed {
		c.lifecycleMtx.Unlock()
		return errControllerClosed
	}
	defer c.lifecycleMtx.Unlock()

	c.mtx.Lock()
	defer c.mtx.Unlock()

	// set startup config
	c.startup.SetState(nextConf.GetStart())

	prevCtrlConf := c.conf.Load()
	prevConf := prevCtrlConf.GetProjectConfig()
	if nextConf.EqualVT(prevConf) {
		return nil
	}

	// update the config
	nextCtrlConf := prevCtrlConf.CloneVT()
	nextCtrlConf.ProjectConfig = nextConf.CloneVT()
	c.conf.Store(nextCtrlConf)
	c.publishProjectConfigStatus(nextConf)

	// build list of running manifest builders
	manifestBuilders := c.getRunningManifestBuilders()

	// build key/value map of seen keys so we know which to cancel
	seenManifestBuilders := make(map[string]struct{}, len(manifestBuilders))
	restartedManifestBuilders := make(map[string]struct{}, len(manifestBuilders))

	// restart any manifest builders that no longer are up-to-date
	nextManifests := nextConf.GetManifests()
	nextRemotes := nextConf.GetRemotes()
	for manifestID, nextManifest := range nextManifests {
		for _, builder := range manifestBuilders {
			// find only builders with this manifest id
			if builder.conf.GetManifestId() != manifestID {
				continue
			}

			// if we already restarted, continue
			if _, ok := restartedManifestBuilders[builder.key]; ok {
				continue
			}

			// if the remote does not exist: continue
			// we will delete the builder below
			remoteConf, remoteConfOk := nextRemotes[builder.conf.RemoteId]
			if !remoteConfOk || remoteConf == nil {
				continue
			}

			// compare the configs and conditionally restart if different
			_, wasReset := c.manifestBuilders.RestartRoutine(
				builder.key,
				func(_ string, trk *manifestBuilderTracker) bool {
					// this includes the case where trkConf is nil (not loaded yet)
					if !trk.manifestConf.Load().EqualVT(nextManifest) {
						return true
					}

					currRemoteConf := trk.remoteConf.Load()
					if currRemoteConf == nil {
						// remote not resolved yet, restart to be sure we pick up any changes.
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

			// mark the builder as seen so we don't cancel it later
			seenManifestBuilders[builder.key] = struct{}{}
		}
	}

	// delete any manifest builders that no longer have corresponding configs
	for _, builder := range manifestBuilders {
		// if the builder was not seen: delete it
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

func (c *Controller) publishProjectConfigStatus(projectConfig *bldr_project.ProjectConfig) {
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
	// build the manifest builder configs
	for _, manifestBuilderConf := range manifestBuilderConfigs {
		if err := manifestBuilderConf.Validate(); err != nil {
			return nil, nil, err
		}
	}

	// add refs
	refs := make([]*ManifestBuilderRef, 0, len(manifestBuilderConfigs))
	defer func() {
		for _, ref := range refs {
			ref.Release()
		}
	}()
	for _, manifestBuilderConfig := range manifestBuilderConfigs {
		ref, err := c.AddManifestBuilderRef(manifestBuilderConfig)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, ref)
	}

	// wait for the manifests to finishing building
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
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	c.lifecycleMtx.Lock()
	if c.closed {
		c.lifecycleMtx.Unlock()
		return nil, errControllerClosed
	}
	c.mtx.Lock()

	projConf := c.conf.Load().GetProjectConfig()
	_, ok := projConf.GetManifests()[conf.GetManifestId()]
	if !ok {
		c.mtx.Unlock()
		c.lifecycleMtx.Unlock()
		return nil, bldr_project.ErrManifestConfNotFound
	}
	_, ok = projConf.GetRemotes()[conf.GetRemoteId()]
	if !ok {
		c.mtx.Unlock()
		c.lifecycleMtx.Unlock()
		return nil, bldr_project.ErrRemoteNotFound
	}

	ref, tracker, _ := c.manifestBuilders.AddKeyRef(conf.MarshalB58())
	c.mtx.Unlock()
	c.lifecycleMtx.Unlock()
	tracker.refreshManifestBuilderStatusMeta()
	return newManifestBuilderRef(ref, tracker), nil
}

// AddRemoteRef adds a reference to a Remote.
// Returns ErrRemoteNotFound if the remote was not found.
func (c *Controller) AddRemoteRef(remoteID string) (*RemoteRef, error) {
	c.lifecycleMtx.Lock()
	defer c.lifecycleMtx.Unlock()

	if c.closed {
		return nil, errControllerClosed
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

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

// AddFetchManifestBuilderRef adds a ManifestBuilderRef for a FetchManifest directive.
func (c *Controller) AddFetchManifestBuilderRef(ctx context.Context, manifestMeta *bldr_manifest.ManifestMeta) (*ManifestBuilderRef, *RemoteRef, error) {
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

	buildType := manifestMeta.GetBuildType()
	if buildType == "" {
		buildType = string(bldr_manifest.BuildType_DEV)
		manifestMeta.BuildType = buildType
	}

	projectConfig := c.conf.Load().GetProjectConfig()
	webPkgDeps := resolveWebPkgDeps(c.le, projectConfig.GetManifests())
	dependencyRefs := make([]*ManifestBuilderRef, 0, len(webPkgDeps[manifestMeta.GetManifestId()]))
	defer func() {
		for _, dependencyRef := range dependencyRefs {
			dependencyRef.Release()
		}
	}()
	for _, dependencyID := range webPkgDeps[manifestMeta.GetManifestId()] {
		dependencyRef, err := c.AddManifestBuilderRef(NewManifestBuilderConfig(
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

	// note: BuildManifests overrides RemoteId with manifestRemoteID
	manifestBuilderRef, err := c.AddManifestBuilderRef(NewManifestBuilderConfig(
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
	c.lifecycleMtx.Lock()
	if c.closed {
		c.lifecycleMtx.Unlock()
		return context.Canceled
	}

	// start the plugin build controllers and remote trackers
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
	c.lifecycleMtx.Lock()
	closed := c.closed
	c.lifecycleMtx.Unlock()
	if closed {
		return nil, nil
	}

	dir := di.GetDirective()
	switch d := dir.(type) {
	case bldr_manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(di, d), nil)
	}

	return nil, nil
}

func (c *Controller) Close() error {
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

	c.manifestBuilders.ClearContext()
	c.remotes.ClearContext()
	c.startup.ClearContext()
	c.routines.Wait()
	close(done)
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)

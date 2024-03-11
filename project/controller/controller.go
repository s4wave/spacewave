package bldr_project_controller

import (
	"context"
	"sync"
	"sync/atomic"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bldr/project"

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
	// mtx guards writing below fields
	mtx sync.Mutex
	// conf is the current controller config
	conf atomic.Pointer[Config]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	ctrl := &Controller{
		le:  le,
		bus: bus,
	}
	ctrl.conf.Store(cc)
	ctrl.manifestBuilders = keyed.NewKeyedRefCountWithLogger(ctrl.newManifestBuilderTracker, le)
	ctrl.remotes = keyed.NewKeyedRefCountWithLogger(ctrl.newRemoteTracker, le)
	return ctrl
}

// GetConfig returns the current config.
func (c *Controller) GetConfig() *Config {
	return c.conf.Load()
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

	c.mtx.Lock()
	defer c.mtx.Unlock()

	prevCtrlConf := c.conf.Load()
	prevConf := prevCtrlConf.GetProjectConfig()
	if nextConf.EqualVT(prevConf) {
		return nil
	}

	// update the config
	nextCtrlConf := prevCtrlConf.CloneVT()
	nextCtrlConf.ProjectConfig = nextConf.CloneVT()
	c.conf.Store(nextCtrlConf)

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

// BuildManifests compiles a set of manifests linking them to the remote object key.
//
// Returns the list of created manifest refs and corresponding object keys.
func (c *Controller) BuildManifests(
	ctx context.Context,
	manifestBuilderConfigs []*ManifestBuilderConfig,
) ([]*bldr_manifest.ManifestRef, []string, error) {
	// add reference to the remote
	/*
		remoteEng, remoteRef, err := c.WaitRemote(ctx, remoteID)
		if err != nil {
			return nil, nil, err
		}
		defer remoteRef.Release()
	*/

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

		// link the manifests to the link keys
	}

	return manifestRefs, manifestObjKeys, nil
}

// AddManifestBuilderRef adds a reference to a manifest compiler.
func (c *Controller) AddManifestBuilderRef(conf *ManifestBuilderConfig) (*ManifestBuilderRef, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	projConf := c.conf.Load().GetProjectConfig()
	_, ok := projConf.GetManifests()[conf.GetManifestId()]
	if !ok {
		return nil, bldr_project.ErrManifestConfNotFound
	}
	_, ok = projConf.GetRemotes()[conf.GetRemoteId()]
	if !ok {
		return nil, bldr_project.ErrRemoteNotFound
	}

	ref, tracker, _ := c.manifestBuilders.AddKeyRef(conf.MarshalB58())
	return newManifestBuilderRef(ref, tracker), nil
}

// AddRemoteRef adds a reference to a Remote.
// Returns ErrRemoteNotFound if the remote was not found.
func (c *Controller) AddRemoteRef(remoteID string) (*RemoteRef, error) {
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

	// note: BuildManifests overrides RemoteId with manifestRemoteID
	manifestBuilderRef, err := c.AddManifestBuilderRef(&ManifestBuilderConfig{
		ManifestId: manifestMeta.GetManifestId(),
		PlatformId: manifestMeta.GetPlatformId(),
		BuildType:  buildType,
		RemoteId:   manifestRemoteID,
	})
	if err != nil {
		remoteRef.Release()
		return nil, nil, err
	}
	return manifestBuilderRef, remoteRef, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// start the plugin build controllers and remote trackers
	c.manifestBuilders.SetContext(ctx, true)
	c.remotes.SetContext(ctx, true)

	// load the startup plugins, if configured
	conf := c.GetConfig()
	projConf := conf.GetProjectConfig()
	loadPluginIDs := projConf.GetStart().GetPlugins()
	if conf.GetStart() && len(loadPluginIDs) != 0 {
		for _, pluginID := range loadPluginIDs {
			c.le.WithField("plugin-id", pluginID).Info("loading startup plugin")
			_, plugRef, err := c.bus.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
			if err != nil {
				return err
			}
			defer plugRef.Release()
		}

		// wait for context cancel to release plugin refs
		<-ctx.Done()
	}

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case bldr_manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, di, d), nil)
	}

	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

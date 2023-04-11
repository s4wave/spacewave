package bldr_project_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/timestamp"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver"
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
	// c is the controller config
	c *Config

	// manifestBuilders is the set of keyed build controllers.
	// NOTE: this will eventually be replaced with Forge jobs.
	// key is the ManifestBuilderConfig object in b58 format.
	manifestBuilders *keyed.KeyedRefCount[string, *manifestBuilderTracker]
	// remotes is the set of keyed remote access controllers.
	remotes *keyed.KeyedRefCount[string, *remoteTracker]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	ctrl := &Controller{
		le:  le,
		bus: bus,
		c:   cc,
	}
	ctrl.manifestBuilders = keyed.NewKeyedRefCountWithLogger(ctrl.newManifestBuilderTracker, le)
	ctrl.remotes = keyed.NewKeyedRefCountWithLogger(ctrl.newRemoteTracker, le)
	return ctrl
}

// GetConfig returns the config.
func (c *Controller) GetConfig() *Config {
	return c.c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr project controller",
	)
}

// BuildManifestBundle compiles a manifest bundle by adding a remote ref & builder refs.
// Writes the bundle to bundleObjKey.
// If a bundle already exists, appends to it (adds manifests).
// The given ManifestBulderConfigs are updated with object keys prefixed by the bundleObjKey.
// If an object key is already set in the ManifestBuilderConfig it will be used instead.
// The object keys in the ManifestBuilderConfigs can be empty.
func (c *Controller) BuildManifestBundle(
	ctx context.Context,
	remoteID, bundleObjKey string,
	manifestBuilderConfigs []*ManifestBuilderConfig,
) (*bldr_manifest.ManifestBundle, *bucket.ObjectRef, error) {
	// add a remote ref
	remoteRef, err := c.AddRemoteRef(remoteID)
	if err != nil {
		return nil, nil, err
	}
	defer remoteRef.Release()

	remoteEngPtr, err := remoteRef.GetResultPromise().Await(ctx)
	if err != nil {
		return nil, nil, err
	}
	remoteEng := *remoteEngPtr

	// build the manifest builder configs
	for _, manifestBuilderConf := range manifestBuilderConfigs {
		manifestID := manifestBuilderConf.GetManifestId()
		if manifestID == "" {
			return nil, nil, bldr_manifest.ErrEmptyManifestID
		}
		manifestBuilderConf.RemoteId = remoteID
		if manifestBuilderConf.GetObjectKey() == "" {
			manifestBuilderConf.ObjectKey = bldr_manifest.NewManifestKey(bundleObjKey, manifestID)
		}
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
	for _, ref := range refs {
		result, err := ref.GetResultPromise().Await(ctx)
		if err != nil {
			return nil, nil, err
		}

		manifestObjKeys = append(manifestObjKeys, result.GetBuilderConfig().GetObjectKey())
		// manifestRefs = append(manifestRefs, result.GetBuilderResult().GetManifestRef())
	}

	// now
	now := timestamp.Now()

	// create the ManifestBundle and link to base object key defined in the remote
	// link the bundle to the link_object_keys as well
	engTx, err := remoteEng.NewTransaction(true)
	if err != nil {
		return nil, nil, err
	}
	defer engTx.Discard()

	manifestBundle, manifestBundleRef, err := bldr_manifest_world.CreateManifestBundle(
		ctx,
		engTx,
		bundleObjKey,
		manifestObjKeys,
		&now,
	)
	if err != nil {
		return nil, nil, err
	}

	// create the links to the additional link keys
	for _, linkObjKey := range remoteRef.GetRemoteConfig().GetLinkObjectKeys() {
		quad := bldr_manifest_world.NewManifestQuad(linkObjKey, bundleObjKey, "")
		if err := engTx.SetGraphQuad(quad); err != nil {
			return nil, nil, err
		}
	}
	c.le.
		WithField("object-key", bundleObjKey).
		Infof("created manifest bundle with %d manifests: %s", len(manifestBundle.GetManifestRefs()), manifestBundleRef.MarshalString())

	err = engTx.Commit(ctx)
	if err != nil {
		return nil, nil, err
	}

	// done
	return manifestBundle, manifestBundleRef, nil
}

// AddManifestBuilderRef adds a reference to a manifest compiler.
func (c *Controller) AddManifestBuilderRef(conf *ManifestBuilderConfig) (*ManifestBuilderRef, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	_, ok := c.c.GetProjectConfig().GetManifests()[conf.GetManifestId()]
	if !ok {
		return nil, bldr_project.ErrManifestConfNotFound
	}
	_, ok = c.c.GetProjectConfig().GetRemotes()[conf.GetRemoteId()]
	if !ok {
		return nil, bldr_project.ErrRemoteNotFound
	}
	ref, tracker, _ := c.manifestBuilders.AddKeyRef(conf.MarshalB58())
	return newManifestBuilderRef(ref, tracker), nil
}

// AddRemoteRef adds a reference to a Remote.
// Returns ErrRemoteNotFound if the remote was not found.
func (c *Controller) AddRemoteRef(remoteID string) (*RemoteRef, error) {
	_, ok := c.c.GetProjectConfig().GetRemotes()[remoteID]
	if !ok {
		return nil, bldr_project.ErrRemoteNotFound
	}
	ref, tracker, _ := c.remotes.AddKeyRef(remoteID)
	return newRemoteRef(ref, tracker), nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// start the plugin build controllers and remote trackers
	c.manifestBuilders.SetContext(ctx, true)
	c.remotes.SetContext(ctx, true)

	// load the startup plugins, if configured
	projConf := c.GetConfig().GetProjectConfig()
	loadPluginIDs := projConf.GetStart().GetPlugins()
	if c.c.GetStart() && len(loadPluginIDs) != 0 {
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

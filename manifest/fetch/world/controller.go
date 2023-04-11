package manifest_fetch_world

import (
	"context"
	"regexp"

	manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/world"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/manifest/fetch/world"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller fetches Manifests via world lookups.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
	// fetchManifestIdRe is the parsed regex to filter manifest by.
	// if nil, accepts any
	fetchManifestIdRe *regexp.Regexp
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	// note: checked in Validate()
	manifestIdRe, _ := conf.ParseFetchManifestIdRegex()
	return &Controller{
		le:                le,
		bus:               bus,
		conf:              conf,
		fetchManifestIdRe: manifestIdRe,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"fetches manifests via world: "+c.conf.GetWorldId(),
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case manifest.FetchManifest:
		return directive.R(c.resolveFetchManifest(ctx, inst, d))
	}
	return nil, nil
}

// FetchManifest fetches a manifest, yielding the FetchManifestResponse.
// if returnIfIdle is set, returns an error if the directive becomes idle (not found)
// Returns nil, nil if not found.
// Returns if context is canceled.
func (c *Controller) FetchManifest(
	rctx context.Context,
	manifestMeta *manifest.ManifestMeta,
	returnIfIdle bool,
) (*manifest.FetchManifestResponse, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	c.le.Debugf("fetching manifest %s via world %s", manifestMeta.GetManifestId(), c.conf.GetWorldId())
	worldEngine, _, worldEngineRef, err := world.ExLookupWorldEngine(ctx, c.bus, returnIfIdle, c.conf.GetWorldId(), ctxCancel)
	if err != nil {
		return nil, err
	}
	defer worldEngineRef.Release()

	tx, err := worldEngine.NewTransaction(false)
	if err != nil {
		return nil, err
	}

	manifests, manifestErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		tx,
		manifestMeta.GetManifestId(),
		manifestMeta.GetPlatformId(),
		c.conf.GetObjectKeys()...,
	)
	tx.Discard()
	if err != nil {
		return nil, err
	}

	for _, err := range manifestErrs {
		c.le.WithError(err).Warn("ignoring invalid manifest")
	}

	// take the first manifest only
	if len(manifests) != 0 {
		selManifest := manifests[0]
		return &manifest.FetchManifestResponse{
			ManifestRef: manifest.NewManifestRef(selManifest.Manifest.Meta, selManifest.ManifestRef),
		}, nil
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

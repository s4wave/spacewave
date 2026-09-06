//go:build !js

package bldr_project_controller

import (
	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

// addFetchManifestBuildRef retains the active build for a dependency tuple.
// A default build is created only when that tuple has no active builder.
func (c *Controller) addFetchManifestBuildRef(conf *ManifestBuilderConfig) (*ManifestBuilderRef, error) {
	refs, err := c.addManifestBuilderRefs([]*ManifestBuilderConfig{conf}, true)
	if err != nil {
		return nil, err
	}
	return refs[0], nil
}

// addManifestBuilderRefs registers a complete set before dependency resolution
// can observe it. Fetches reuse active configurations; explicit builds keep
// their declared inputs. Returned references retain the normal keyed lifetime.
func (c *Controller) addManifestBuilderRefs(confs []*ManifestBuilderConfig, reuseActive bool) ([]*ManifestBuilderRef, error) {
	// Validate before acquiring the controller's lifecycle and registry locks.
	for _, conf := range confs {
		if err := conf.Validate(); err != nil {
			return nil, err
		}
	}

	// Registration and lookup share the existing lock order used by shutdown.
	c.lifecycleMtx.Lock()
	c.mtx.Lock()
	refs, err := c.addManifestBuilderRefsLocked(confs, reuseActive)
	c.mtx.Unlock()
	c.lifecycleMtx.Unlock()
	if err != nil {
		return nil, err
	}

	// Status callbacks may reenter the controller, so publish outside its locks.
	for _, ref := range refs {
		ref.tracker.refreshManifestBuilderStatusMeta()
	}
	return refs, nil
}

// addManifestBuilderRefsLocked selects and registers builders with lifecycleMtx
// and mtx held. It validates the whole set before creating any references.
func (c *Controller) addManifestBuilderRefsLocked(confs []*ManifestBuilderConfig, reuseActive bool) ([]*ManifestBuilderRef, error) {
	if c.closed {
		return nil, errControllerClosed
	}

	// Resolve every input first so a failed request leaves no partial build set.
	projectConfig := c.conf.Load().GetProjectConfig()
	selected := make([]*ManifestBuilderConfig, len(confs))
	for i, conf := range confs {
		if _, ok := projectConfig.GetManifests()[conf.GetManifestId()]; !ok {
			return nil, bldr_project.ErrManifestConfNotFound
		}
		if _, ok := projectConfig.GetRemotes()[conf.GetRemoteId()]; !ok {
			return nil, bldr_project.ErrRemoteNotFound
		}
		selected[i] = conf
		if reuseActive {
			active, err := c.findActiveManifestBuildConfig(conf)
			if err != nil {
				return nil, err
			}
			if active != nil {
				selected[i] = active
			}
		}
	}

	// The keyed registry remains the sole owner of build execution and release.
	refs := make([]*ManifestBuilderRef, 0, len(selected))
	for _, conf := range selected {
		ref, tracker, _ := c.manifestBuilders.AddKeyRef(conf.MarshalB58())
		refs = append(refs, newManifestBuilderRef(ref, tracker))
	}
	return refs, nil
}

// findActiveManifestBuildConfig selects an unambiguous active dependency build.
// FetchManifest carries no build-target discriminator, so conflicting executable
// inputs are errors. TargetPlatformIds is informational and does not affect them.
func (c *Controller) findActiveManifestBuildConfig(want *ManifestBuilderConfig) (*ManifestBuilderConfig, error) {
	var selected *ManifestBuilderConfig
	for _, builder := range c.getRunningManifestBuilders() {
		conf := builder.conf
		if conf.GetManifestId() != want.GetManifestId() ||
			conf.GetBuildType() != want.GetBuildType() ||
			conf.GetPlatformId() != want.GetPlatformId() ||
			conf.GetRemoteId() != want.GetRemoteId() {
			continue
		}
		if selected != nil &&
			(!selected.GetBuilderConfigOverride().EqualVT(conf.GetBuilderConfigOverride()) ||
				!selected.GetBuildPolicy().EqualVT(conf.GetBuildPolicy())) {
			return nil, errors.Errorf("conflicting active builds for %s@%s (%s)", want.GetManifestId(), want.GetPlatformId(), want.GetBuildType())
		}
		selected = conf
	}
	return selected, nil
}

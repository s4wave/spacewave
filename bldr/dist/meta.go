package bldr_dist

import (
	"slices"

	"github.com/klauspost/compress/s2"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/util/labels"
)

const (
	// EntrypointRoleDesktop identifies the native desktop UI entrypoint.
	EntrypointRoleDesktop = "desktop"
	// EntrypointRoleBrowser identifies the browser/CDN entrypoint.
	EntrypointRoleBrowser = "browser"
	// EntrypointRoleCLI identifies the managed CLI entrypoint.
	EntrypointRoleCLI = "cli"
)

// NewDistMeta constructs a new DistMeta.
func NewDistMeta(projectID, platformID string, startupPlugins []string, distWorldRef *bucket.ObjectRef, distObjKey string) *DistMeta {
	return &DistMeta{
		ProjectId:      projectID,
		PlatformId:     platformID,
		StartupPlugins: startupPlugins,
		DistWorldRef:   distWorldRef,
		DistObjectKey:  distObjKey,
	}
}

// NewDistEntrypointMeta constructs a new DistMeta with entrypoint identity.
func NewDistEntrypointMeta(
	projectID string,
	platformID string,
	startupPlugins []string,
	distWorldRef *bucket.ObjectRef,
	distObjKey string,
	entrypointRole string,
	channelKey string,
	manifestID string,
	manifestRev uint64,
) *DistMeta {
	meta := NewDistMeta(projectID, platformID, startupPlugins, distWorldRef, distObjKey)
	meta.EntrypointRole = entrypointRole
	meta.ChannelKey = channelKey
	meta.ManifestId = manifestID
	meta.ManifestRev = manifestRev
	return meta
}

// UnmarshalDistMetaB58 unmarshals a b58 dist meta.
// Note: we compress with s2 compression.
func UnmarshalDistMetaB58(str string) (*DistMeta, error) {
	m := &DistMeta{}
	data, err := b58.Decode(str)
	if err != nil {
		return nil, err
	}
	data, err = s2.Decode(nil, data)
	if err != nil {
		return nil, err
	}
	if err := m.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate checks the dist meta.
func (m *DistMeta) Validate() error {
	if err := labels.ValidateDNSLabel(m.GetProjectId()); err != nil {
		return errors.Wrap(err, "project_id")
	}
	if _, err := bldr_platform.ParsePlatform(m.GetPlatformId()); err != nil {
		return errors.Wrap(err, "platform_id")
	}
	if err := m.GetDistWorldRef().Validate(); err != nil {
		return errors.Wrap(err, "dist_world_ref")
	}
	if m.GetDistObjectKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "dist_object_key")
	}
	if role := m.GetEntrypointRole(); role != "" && !slices.Contains(validEntrypointRoles(), role) {
		return errors.Errorf("entrypoint_role: invalid role %q", role)
	}
	return nil
}

// MarshalB58 marshals the conf to a b58 string.
// note: we compress with s2 compression.
func (m *DistMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	dat = s2.EncodeBest(nil, dat)
	return b58.Encode(dat)
}

func validEntrypointRoles() []string {
	return []string{EntrypointRoleDesktop, EntrypointRoleBrowser, EntrypointRoleCLI}
}

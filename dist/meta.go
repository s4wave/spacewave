package bldr_dist

import (
	"github.com/aperturerobotics/bifrost/util/labels"
	"github.com/klauspost/compress/s2"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
)

// NewDistMeta constructs a new DistMeta.
func NewDistMeta(projectID, platformID string, startupPlugins []string) *DistMeta {
	return &DistMeta{
		ProjectId:      projectID,
		PlatformId:     platformID,
		StartupPlugins: startupPlugins,
	}
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
	return nil
}

// MarshalB58 marshals the conf to a b58 string.
// note: we compress with s2 compression.
func (m *DistMeta) MarshalB58() string {
	dat, _ := m.MarshalVT()
	dat = s2.EncodeBest(nil, dat)
	return b58.Encode(dat)
}

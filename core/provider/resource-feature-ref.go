package provider

import (
	"github.com/sirupsen/logrus"
)

// MaxProviderFeatureMetaSize is the max size of the ProviderFeatureMeta field.
const MaxProviderFeatureMetaSize = 12000

// Validate validates the shared object ref.
func (r *ProviderFeatureResourceRef) Validate() error {
	if err := r.GetProviderResourceRef().Validate(); err != nil {
		return err
	}
	if len(r.GetProviderFeatureMeta()) > MaxProviderFeatureMetaSize {
		return ErrProviderFeatureMetaSizeExceeded
	}
	return nil
}

// GetLogger adds debug values to the logger.
func (r *ProviderFeatureResourceRef) GetLogger(le *logrus.Entry) *logrus.Entry {
	return r.
		GetProviderResourceRef().
		GetLogger(le)
}

//go:build goscript

package resource_sobject

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/core/sobject"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

var errMountSharedObjectBodyUnavailable = errors.New("shared object body mounting is not supported in goscript")

// MountSharedObjectBody reports that body mounting is unavailable in GoScript.
func (r *SharedObjectResource) MountSharedObjectBody(_ context.Context, _ *s4wave_sobject.MountSharedObjectBodyRequest) (*s4wave_sobject.MountSharedObjectBodyResponse, error) {
	return nil, sobject.WrapSharedObjectHealthError(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		errMountSharedObjectBodyUnavailable,
	)
}

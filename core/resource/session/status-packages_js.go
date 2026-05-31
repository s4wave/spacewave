//go:build js

package resource_session

import (
	"context"

	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

func (r *StatusResource) watchRecoveryPackageChanges(ctx context.Context, notify func()) {
}

func (r *StatusResource) buildNativePackageRecoveryStatuses() []*s4wave_status.NativePackageRecoveryStatus {
	return nil
}

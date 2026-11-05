//go:build !js

package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// commitAdmitTest commits tx through the SharedObject engine's admission.
func commitAdmitTest(ctx context.Context, tx world.Tx) (world.Admission, error) {
	return tx.(interface {
		CommitAdmit(context.Context) (world.Admission, error)
	}).CommitAdmit(ctx)
}

// rootRetainingTestEngine is the SharedObject engine's root retention.
type rootRetainingTestEngine interface {
	// SetRetainedRoot retains ref under name.
	SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error
}

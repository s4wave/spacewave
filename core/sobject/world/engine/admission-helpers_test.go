//go:build !js

package sobject_world_engine_test

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
)

// commitAdmitTest commits tx through the SharedObject engine's admission.
func commitAdmitTest(ctx context.Context, tx world.Tx) (world.Admission, error) {
	return tx.(interface {
		CommitAdmit(context.Context) (world.Admission, error)
	}).CommitAdmit(ctx)
}

// onReinstallTest registers fn with the SharedObject engine's reinstall hooks.
func onReinstallTest(ctx context.Context, eng world.Engine, fn func(seqno uint64)) (func(), error) {
	return eng.(interface {
		OnReinstall(context.Context, func(uint64)) (func(), error)
	}).OnReinstall(ctx, fn)
}

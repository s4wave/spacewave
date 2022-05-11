package entrypoint

import (
	"context"

	"github.com/aperturerobotics/bldr/storage"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/sirupsen/logrus"
)

// ExecuteStorage runs storage from a list of default providers.
//
// returns a release function. logs & ignores any errors.
func ExecuteStorage(ctx context.Context, b bus.Bus, le *logrus.Entry, storageProviders []storage.Storage) func() {
	le.Debugf("executing %d storage provider(s)", len(storageProviders))

	relFns := make([]func(), 0, len(storageProviders))
	for _, st := range storageProviders {
		vc := st.BuildVolumeConfig("aperture")
		_, _, volRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(vc),
			nil,
		)
		if err != nil {
			le.
				WithError(err).
				Warn("unable to start volume controller, skipping")
		} else {
			relFns = append(relFns, volRef.Release)
		}
	}

	return func() {
		for _, fn := range relFns {
			fn()
		}
	}
}

package storage

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/sirupsen/logrus"
)

// Storage is an available storage mechanism in the environment.
type Storage interface {
	// GetStorageInfo returns StorageInfo.
	GetStorageInfo() *StorageInfo
	// AddFactories adds the factories to the resolver.
	AddFactories(b bus.Bus, sr *static.Resolver)
	// BuildVolumeConfig creates the volume config for the store ID.
	// Returns nil if the storage cannot produce Volume.
	BuildVolumeConfig(id string) config.Config
}

// ExecuteStorage runs storage from a list of default providers.
//
// returns a release function. logs & ignores any errors.
func ExecuteStorage(ctx context.Context, b bus.Bus, le *logrus.Entry, storageProviders []Storage) func() {
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

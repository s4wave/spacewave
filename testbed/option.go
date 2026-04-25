package testbed

import "github.com/s4wave/spacewave/bldr/storage"

// Option is an option passed to NewTestbed.
type Option any

type withWorldVerbose struct{ verbose bool }

// WithWorldVerbose logs all world engine operations.
func WithWorldVerbose(verbose bool) Option {
	return &withWorldVerbose{verbose: verbose}
}

type withStorages struct{ storages []storage.Storage }

// WithStorages overrides the storage backends used by the testbed storage controller.
func WithStorages(storages ...storage.Storage) Option {
	return &withStorages{storages: storages}
}

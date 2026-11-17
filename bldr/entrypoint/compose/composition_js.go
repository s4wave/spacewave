//go:build js

package compose

import "github.com/s4wave/spacewave/bldr/storage"

// Composition is the project-supplied part of a browser host process. Browser
// hosts have no command line, so the composition carries only factories.
type Composition struct {
	// Factories construct the project's controller factories on each host bus.
	Factories []AddFactoriesFunc
}

// ConfigureStorage returns storages unchanged: a browser host configures no
// Volumes.
func (c *Composition) ConfigureStorage(storages []storage.Storage) []storage.Storage {
	return storages
}

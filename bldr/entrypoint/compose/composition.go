//go:build !js

package compose

import (
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/bldr/storage"
)

// Composition is the project-supplied part of a native host process.
type Composition struct {
	// NativeAction replaces the default no-argument distribution launch. It runs
	// before writable bus construction and independently of NativeRunner.
	NativeAction NativeAction
	// Factories construct the project's controller factories on each host bus.
	Factories []AddFactoriesFunc
	// Commands build the project's CLI commands.
	Commands []cli_entrypoint.BuildCommandsFunc
	// Flags are the project's root CLI flags, accepted before any command.
	Flags []cli.Flag
	// ConfigureVolume adjusts each Volume config the host's storage builds.
	// The bus is built after the command line is parsed, so it may read the
	// project's flags.
	ConfigureVolume storage.ConfigureVolumeFunc
	// Distribution is set by a distribution entrypoint's generated main, the
	// app a person runs in dev and release builds alike, after Compose
	// returns and before the process runs, so the project's hooks read it
	// when they run and not when Compose builds them.
	Distribution bool
}

// ConfigureStorage returns storages with each Volume config they build passed
// through ConfigureVolume. A nil composition or hook returns them unchanged.
func (c *Composition) ConfigureStorage(storages []storage.Storage) []storage.Storage {
	if c == nil {
		return storages
	}
	return storage.ConfigureAll(storages, c.ConfigureVolume)
}

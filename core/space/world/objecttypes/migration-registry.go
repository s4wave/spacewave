//go:build !tinygo && !goscript

package objecttypes

import space_migration "github.com/s4wave/spacewave/core/space/migration"

// MigrationRegistry returns the classification registry for central ObjectTypes.
func MigrationRegistry() (*space_migration.Registry, error) {
	return space_migration.BuiltInRegistry()
}

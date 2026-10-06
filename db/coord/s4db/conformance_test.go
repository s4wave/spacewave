//go:build !js && !wasip1

package s4db

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	db_s4db "github.com/s4wave/spacewave/db/s4db"
)

func TestCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(tb testing.TB) (coord.Coordinator, coord.Coordinator) {
		db := openTestDB(tb, filepath.Join(tb.TempDir(), "volume.s4wave"))
		inner := coord_inmem.NewCoordinator()
		return NewCoordinator(db, inner), NewCoordinator(db, inner)
	})
}

// openTestDB opens the database at path and closes it after tb.
func openTestDB(tb testing.TB, path string) *db_s4db.DB {
	// Open the file.
	tb.Helper()
	db, err := db_s4db.Open(path, db_s4db.Options{})
	if err != nil {
		tb.Fatal(err)
	}

	// Close it once the test ends.
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Error(err)
		}
	})
	return db
}

package filelock

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

func TestCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(tb testing.TB) (coord.Coordinator, coord.Coordinator) {
		dir := ""
		storeID := tb.Name()
		if lockFilesSupported {
			dir = tb.TempDir()
			storeID = filepath.Join(dir, "volume.db")
		}
		inner := coord_inmem.NewCoordinator()
		return NewCoordinator(dir, storeID, inner), NewCoordinator(dir, storeID, inner)
	})
}

package s4wave_git

import (
	"os"
	"testing"
	"time"
)

// TestImportLocalRepoToRefRepeatMeasurement measures two imports of an opt-in local fixture.
func TestImportLocalRepoToRefRepeatMeasurement(t *testing.T) {
	// Require an explicitly supplied fixture so ordinary checks stay small.
	source := os.Getenv("SPACEWAVE_IMPORT_MEASURE_SOURCE")
	if source == "" {
		t.Skip("set SPACEWAVE_IMPORT_MEASURE_SOURCE to a disposable repository clone")
	}
	ctx, ws := localImportWorld(t)

	// Measure the first import into an empty World.
	start := time.Now()
	first, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	initial := time.Since(start)

	// Describe the committed fixture through the stored pack metadata.
	packs := importedPacks(t, ctx, ws, first)
	var objects, packBytes, indexBytes uint64
	for _, pack := range packs {
		objects += pack.GetObjectCount()
		packBytes += pack.GetPackSize()
		indexBytes += pack.GetIdxSize()
	}

	// Measure a second import after the World already holds the committed graph.
	start = time.Now()
	second, _, _, err := ImportLocalRepoToRef(ctx, ws, source)
	if err != nil {
		t.Fatal(err)
	}
	repeated := time.Since(start)
	if !first.GetRootRef().EqualsRef(second.GetRootRef()) {
		t.Fatal("repeat import changed the repository snapshot")
	}
	t.Logf("objects: %d; pack bytes: %d; index bytes: %d; first import: %s; second import: %s; same snapshot: true", objects, packBytes, indexBytes, initial, repeated)
}

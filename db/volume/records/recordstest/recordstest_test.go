package recordstest

import (
	"testing"

	"github.com/s4wave/spacewave/db/volume/records"
)

// TestMemory checks the Store contract on the memory store.
func TestMemory(t *testing.T) {
	if err := Check(t.Context(), records.NewMemory()); err != nil {
		t.Fatal(err)
	}
}

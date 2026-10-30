//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestValueFlowsByFamily checks that the volume reports the values its file
// stored by key family, so a block lands under "block" and not "other".
func TestValueFlowsByFamily(t *testing.T) {
	// Open a volume and store a block too long for the index.
	ctx := t.Context()
	conf := &volume_s4db.Config{Path: filepath.Join(t.TempDir(), "volume.s4wave")}
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), conf)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	data := bytes.Repeat([]byte("flows "), 1024)
	if _, _, err := vol.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}

	// The families come in order, and the block's bytes are under "block".
	values := vol.GetFlows().Values
	var names []string
	for _, v := range values {
		names = append(names, v.Family)
	}
	if len(values) != 4 || names[0] != "block" || names[1] != "object" || names[2] != "graph" || names[3] != "other" {
		t.Fatalf("families = %v; want block, object, graph, other", names)
	}
	if values[0].Written != uint64(len(data)) || values[0].Freed != 0 || values[3].Written >= uint64(len(data)) {
		t.Fatalf("flows = %+v; want the %d block bytes under block", values, len(data))
	}
}

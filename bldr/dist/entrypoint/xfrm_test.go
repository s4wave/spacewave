package dist_entrypoint

import (
	"testing"

	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
)

func TestBuildStorageTransformConfOmitsBlockEnc(t *testing.T) {
	for _, step := range buildStorageTransformConf("spacewave") {
		if step.GetConfigID() == transform_blockenc.ConfigID {
			t.Fatal("storage transform config must not include blockenc")
		}
	}
}

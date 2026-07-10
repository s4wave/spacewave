package dist_entrypoint

import (
	"testing"

	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
)

func TestBuildStorageTransformConfOmitsRedundantSteps(t *testing.T) {
	for _, step := range buildStorageTransformConf("spacewave") {
		switch step.GetConfigID() {
		case transform_blockenc.ConfigID:
			t.Fatal("storage transform config must not include blockenc")
		case transform_chksum.ConfigID:
			t.Fatal("content-addressed storage transform config must not include chksum")
		}
	}
}

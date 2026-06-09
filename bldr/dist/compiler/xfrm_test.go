package bldr_dist_compiler

import (
	"testing"

	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
)

func TestBuildEmbedTransformConfOmitsBlockEnc(t *testing.T) {
	for _, step := range buildEmbedTransformConf("spacewave") {
		if step.GetConfigID() == transform_blockenc.ConfigID {
			t.Fatal("embed transform config must not include blockenc")
		}
	}
}

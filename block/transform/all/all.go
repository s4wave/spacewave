package transform_all

import (
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/block/transform/chksum"
	"github.com/aperturerobotics/hydra/block/transform/snappy"
)

// BuildFactories returns the set of all hydra block transforms.
func BuildFactories() []block_transform.StepFactory {
	return []block_transform.StepFactory{
		transform_snappy.NewFactory(),
		transform_chksum.NewFactory(),
	}
}

// BuildFactorySet builds a step factory set.
func BuildFactorySet() (*block_transform.StepFactorySet, error) {
	sfs := block_transform.NewStepFactorySet()
	for _, f := range BuildFactories() {
		sfs.AddFactory(f)
	}
	return sfs, nil
}

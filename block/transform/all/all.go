package transform_all

import (
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	transform_snappy "github.com/aperturerobotics/hydra/block/transform/snappy"
)

// BuildFactories returns the set of all hydra block transforms.
func BuildFactories() []block_transform.StepFactory {
	return []block_transform.StepFactory{
		transform_snappy.NewFactory(),
		transform_s2.NewFactory(),
		transform_chksum.NewFactory(),
		transform_blockenc.NewFactory(),
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

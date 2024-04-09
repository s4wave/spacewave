package transform_all

import (
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_lz4 "github.com/aperturerobotics/hydra/block/transform/lz4"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
)

// BuildStepFactories returns the set of all hydra block transforms.
func BuildStepFactories() []block_transform.StepFactory {
	return []block_transform.StepFactory{
		transform_lz4.NewStepFactory(),
		transform_s2.NewStepFactory(),
		transform_chksum.NewStepFactory(),
		transform_blockenc.NewStepFactory(),
	}
}

// BuildFactorySet builds a step factory set.
func BuildFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	for _, f := range BuildStepFactories() {
		sfs.AddStepFactory(f)
	}
	return sfs
}

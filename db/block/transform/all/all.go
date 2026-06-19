package transform_all

import (
	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// BuildFactorySet builds a step factory set.
func BuildFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	for _, f := range BuildStepFactories() {
		sfs.AddStepFactory(f)
	}
	return sfs
}

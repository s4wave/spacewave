package block_transform

import (
	"sync"
)

// StepFactorySet is a statically compiled set of transformers.
type StepFactorySet struct {
	// mtx guards the map
	mtx sync.Mutex
	// factories contains factories keyed by config id
	factories map[string]StepFactory
}

// NewStepFactorySet constructs a new step factory set.
func NewStepFactorySet() *StepFactorySet {
	return &StepFactorySet{
		factories: make(map[string]StepFactory),
	}
}

// AddFactory attaches a step factory.
func (s *StepFactorySet) AddFactory(f StepFactory) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	cid := f.GetConfigID()
	s.factories[cid] = f
}

// GetFactoryByConfigID returns the factory matching the config id.
// Returns nil if not found.
func (s *StepFactorySet) GetFactoryByConfigID(id string) StepFactory {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.factories[id]
}

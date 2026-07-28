package block_transform

import (
	"sync"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
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

// AddStepFactory attaches a step factory.
func (s *StepFactorySet) AddStepFactory(f StepFactory) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	cid := f.GetConfigID()
	s.factories[cid] = f
}

// GetStepFactoryByConfigID returns the factory matching the config id.
// Returns nil if not found.
func (s *StepFactorySet) GetStepFactoryByConfigID(id string) StepFactory {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.factories[id]
}

// UnmarshalStepConfig unmarshals a StepConfig to a configuration.
//
// Constructs and parses the configuration and returns the config and step factory.
func (s *StepFactorySet) UnmarshalStepConfig(conf *StepConfig) (config.Config, StepFactory, error) {
	tf := s.GetStepFactoryByConfigID(conf.GetId())
	if tf == nil {
		return nil, nil, errors.Errorf(
			"transform unknown: %s",
			conf.GetId(),
		)
	}
	cc := tf.ConstructConfig()
	if err := UnmarshalStepConfig(conf.GetConfig(), cc); err != nil {
		return nil, tf, err
	}
	return cc, tf, nil
}

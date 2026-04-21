package simulate

// SimulatorOption is an option passed to NewSimulator.
type SimulatorOption func(s *Simulator) error

// WithVerbose enables verbose logging.
func WithVerbose() SimulatorOption {
	return func(s *Simulator) error {
		s.verbose = true
		return nil
	}
}

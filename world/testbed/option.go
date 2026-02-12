package testbed

// Option is a option passed to NewTestbed
type Option any

type withWorldVerbose struct{ verbose bool }

// WithWorldVerbose logs all world engine operations.
func WithWorldVerbose(verbose bool) Option {
	return &withWorldVerbose{verbose: verbose}
}

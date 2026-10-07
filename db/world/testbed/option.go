package testbed

// Option is an option passed to NewTestbed.
type Option any

type withWorldVerbose struct{ verbose bool }

// WithWorldVerbose logs all world engine operations.
func WithWorldVerbose(verbose bool) Option {
	return &withWorldVerbose{verbose: verbose}
}

type withChangelog struct{}

// WithChangelog records the World changelog, so WatchChanges reports the keys
// and quads each revision changed.
func WithChangelog() Option {
	return &withChangelog{}
}

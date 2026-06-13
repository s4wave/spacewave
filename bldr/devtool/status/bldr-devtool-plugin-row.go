package status

// BldrDevtoolPluginState describes plugin status.
type BldrDevtoolPluginState int32

const (
	// BldrDevtoolPluginStateUnknown leaves plugin state unset.
	BldrDevtoolPluginStateUnknown BldrDevtoolPluginState = iota
	// BldrDevtoolPluginStateRequested means the plugin has been requested.
	BldrDevtoolPluginStateRequested
	// BldrDevtoolPluginStateRunning means the plugin is running.
	BldrDevtoolPluginStateRunning
	// BldrDevtoolPluginStateErrored means the plugin failed.
	BldrDevtoolPluginStateErrored
)

// String returns the stable display value.
func (s BldrDevtoolPluginState) String() string {
	switch s {
	case BldrDevtoolPluginStateRequested:
		return "requested"
	case BldrDevtoolPluginStateRunning:
		return "running"
	case BldrDevtoolPluginStateErrored:
		return "errored"
	default:
		return "unknown"
	}
}

// BldrDevtoolPluginRow describes one plugin instance row.
type BldrDevtoolPluginRow struct {
	ID          string
	PluginID    string
	InstanceKey string
	State       BldrDevtoolPluginState
	Summary     string
	Error       string
	LastErrorAt string
}

func bldrDevtoolPluginRowEqual(a, b BldrDevtoolPluginRow) bool {
	return a.ID == b.ID &&
		a.PluginID == b.PluginID &&
		a.InstanceKey == b.InstanceKey &&
		a.State == b.State &&
		a.Summary == b.Summary &&
		a.Error == b.Error &&
		a.LastErrorAt == b.LastErrorAt
}

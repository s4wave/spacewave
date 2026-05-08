package status

// BldrDevtoolControllerState describes controller load and exec status.
type BldrDevtoolControllerState int32

const (
	// BldrDevtoolControllerStateUnknown leaves controller state unset.
	BldrDevtoolControllerStateUnknown BldrDevtoolControllerState = iota
	// BldrDevtoolControllerStateRequested means the controller has been requested.
	BldrDevtoolControllerStateRequested
	// BldrDevtoolControllerStateRunning means the controller is running.
	BldrDevtoolControllerStateRunning
	// BldrDevtoolControllerStateIdle means the directive is idle without a running controller.
	BldrDevtoolControllerStateIdle
	// BldrDevtoolControllerStateError means controller load or execution failed.
	BldrDevtoolControllerStateError
)

// String returns the stable display value.
func (s BldrDevtoolControllerState) String() string {
	switch s {
	case BldrDevtoolControllerStateRequested:
		return "requested"
	case BldrDevtoolControllerStateRunning:
		return "running"
	case BldrDevtoolControllerStateIdle:
		return "idle"
	case BldrDevtoolControllerStateError:
		return "error"
	default:
		return "unknown"
	}
}

// BldrDevtoolControllerRow describes one controller load or exec directive row.
type BldrDevtoolControllerRow struct {
	ID           string
	ControllerID string
	Kind         string
	State        BldrDevtoolControllerState
	Summary      string
	Error        string
}

func bldrDevtoolControllerRowEqual(a, b BldrDevtoolControllerRow) bool {
	return a.ID == b.ID &&
		a.ControllerID == b.ControllerID &&
		a.Kind == b.Kind &&
		a.State == b.State &&
		a.Summary == b.Summary &&
		a.Error == b.Error
}

package status

// BldrDevtoolCommandState describes the high-level command lifecycle.
type BldrDevtoolCommandState int32

const (
	// BldrDevtoolCommandStateUnknown leaves command state unset.
	BldrDevtoolCommandStateUnknown BldrDevtoolCommandState = iota
	// BldrDevtoolCommandStateStarting means command startup is running.
	BldrDevtoolCommandStateStarting
	// BldrDevtoolCommandStateRunning means the command is active.
	BldrDevtoolCommandStateRunning
	// BldrDevtoolCommandStateDone means the command completed successfully.
	BldrDevtoolCommandStateDone
	// BldrDevtoolCommandStateError means the command failed.
	BldrDevtoolCommandStateError
	// BldrDevtoolCommandStateCanceled means the command was canceled.
	BldrDevtoolCommandStateCanceled
)

// String returns the stable display value.
func (s BldrDevtoolCommandState) String() string {
	switch s {
	case BldrDevtoolCommandStateStarting:
		return "starting"
	case BldrDevtoolCommandStateRunning:
		return "running"
	case BldrDevtoolCommandStateDone:
		return "done"
	case BldrDevtoolCommandStateError:
		return "error"
	case BldrDevtoolCommandStateCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// BldrDevtoolCommandStatus describes the active bldr command.
type BldrDevtoolCommandStatus struct {
	Name    string
	State   BldrDevtoolCommandState
	Summary string
	Error   string
	LogFile string
}

// IsTerminal returns true when the command cannot make more progress.
func (s BldrDevtoolCommandStatus) IsTerminal() bool {
	return s.State == BldrDevtoolCommandStateDone ||
		s.State == BldrDevtoolCommandStateError ||
		s.State == BldrDevtoolCommandStateCanceled
}

func bldrDevtoolCommandStatusEqual(a, b BldrDevtoolCommandStatus) bool {
	return a.Name == b.Name &&
		a.State == b.State &&
		a.Summary == b.Summary &&
		a.Error == b.Error &&
		a.LogFile == b.LogFile
}

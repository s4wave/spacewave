package status

// BldrDevtoolAttentionSeverity describes the urgency of an attention row.
type BldrDevtoolAttentionSeverity int32

const (
	// BldrDevtoolAttentionSeverityUnknown leaves severity unset.
	BldrDevtoolAttentionSeverityUnknown BldrDevtoolAttentionSeverity = iota
	// BldrDevtoolAttentionSeverityInfo marks informational attention.
	BldrDevtoolAttentionSeverityInfo
	// BldrDevtoolAttentionSeverityWarning marks recoverable attention.
	BldrDevtoolAttentionSeverityWarning
	// BldrDevtoolAttentionSeverityError marks blocking attention.
	BldrDevtoolAttentionSeverityError
)

// String returns the stable display value.
func (s BldrDevtoolAttentionSeverity) String() string {
	switch s {
	case BldrDevtoolAttentionSeverityInfo:
		return "info"
	case BldrDevtoolAttentionSeverityWarning:
		return "warning"
	case BldrDevtoolAttentionSeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// BldrDevtoolAttentionRow describes one recent attention or error item.
type BldrDevtoolAttentionRow struct {
	ID       string
	Source   string
	Message  string
	Detail   string
	Severity BldrDevtoolAttentionSeverity
}

func bldrDevtoolAttentionRowEqual(a, b BldrDevtoolAttentionRow) bool {
	return a.ID == b.ID &&
		a.Source == b.Source &&
		a.Message == b.Message &&
		a.Detail == b.Detail &&
		a.Severity == b.Severity
}

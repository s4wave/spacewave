package selfenrollmentrun

// Summary describes the pending shared objects for a self-enrollment run.
type Summary interface {
	GetIDs() []string
	GetGenerationKey() string
	GetCount() uint32
}

// Request is the durable input to a visible self-enrollment run.
type Request struct {
	GenerationKey string
	IDs           []string
}

// NewRequest builds an owned request snapshot from summary.
func NewRequest(summary Summary) *Request {
	if summary == nil || summary.GetCount() == 0 {
		return nil
	}
	return &Request{
		GenerationKey: summary.GetGenerationKey(),
		IDs:           append([]string(nil), summary.GetIDs()...),
	}
}

// ShouldAutoStart reports whether a pending summary should schedule a run.
func ShouldAutoStart(
	summary Summary,
	skippedGenerationKey string,
	routineReady bool,
	unlockedKeys int,
) bool {
	if !routineReady || summary == nil || summary.GetCount() == 0 {
		return false
	}
	if skippedGenerationKey != "" && skippedGenerationKey == summary.GetGenerationKey() {
		return false
	}
	return unlockedKeys != 0
}

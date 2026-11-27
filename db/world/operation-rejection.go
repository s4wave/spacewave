package world

// OperationRejection reports a deterministic application rejection. The World
// transaction owner discards all changes. Callers may present Message directly.
type OperationRejection struct {
	Code    string
	Message string
}

// Error returns the safe application explanation.
func (e *OperationRejection) Error() string { return e.Message }

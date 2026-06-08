package forge_lib_git_lazyrepo

import "strings"

// ProvenanceError reports a mutation path that cannot be tied to a mounted Repo.
type ProvenanceError struct {
	// Operation is the attempted filesystem operation.
	Operation string
	// MutationPath is the normalized path that was going to be mutated.
	MutationPath string
	// EvidenceObjectKey points at raw provider Evidence when one was recorded.
	EvidenceObjectKey string
	// Reason explains the provenance failure.
	Reason string
}

// Error returns a human-readable error.
func (e *ProvenanceError) Error() string {
	if e == nil {
		return ""
	}
	reason := e.Reason
	if reason == "" {
		reason = "unresolved repo mount provenance"
	}
	if e.EvidenceObjectKey != "" {
		return strings.Join([]string{"forge git lazy repo: ", e.Operation, " for ", e.MutationPath, ": ", reason, " (evidence ", e.EvidenceObjectKey, ")"}, "")
	}
	return strings.Join([]string{"forge git lazy repo: ", e.Operation, " for ", e.MutationPath, ": ", reason}, "")
}

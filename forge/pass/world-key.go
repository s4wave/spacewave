package forge_pass

import (
	"strings"
)

// BuildPassExecutionObjKey builds the object key for a pass execution.
// execPeerID must be set
func BuildPassExecutionObjKey(passObjKey, execPeerID string) string {
	return strings.Join([]string{passObjKey, "exec", execPeerID}, "/")
}

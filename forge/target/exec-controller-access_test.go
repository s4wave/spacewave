package forge_target

import "testing"

// TestAccessHandleHasNoExecutionClaim preserves access-only callers without
// fabricating an Execution claim for their synthetic identity.
func TestAccessHandleHasNoExecutionClaim(t *testing.T) {
	handle := ExecControllerHandleWithAccess("access/test", "", nil, nil, nil)
	if handle.GetExecutionObjectKey() != "access/test" || handle.GetExecutionClaimEpoch() != 0 {
		t.Fatalf("access-only execution = %q epoch %d", handle.GetExecutionObjectKey(), handle.GetExecutionClaimEpoch())
	}
}

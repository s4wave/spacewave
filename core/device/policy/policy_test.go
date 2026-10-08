package device_policy

import (
	"strings"
	"testing"
)

// TestNodeTypeAllowList pins the allow list: a policy file with node type IDs
// reads back unchanged, and an empty or duplicate ID is rejected.
func TestNodeTypeAllowList(t *testing.T) {
	// Write and read back a policy with an allow list.
	stateRoot := t.TempDir()
	policy := &DevicePolicy{
		Revision:   1,
		NodeTypeId: []string{"tcp-port", "local-port"},
	}
	if err := WriteFile(stateRoot, policy); err != nil {
		t.Fatal(err)
	}
	read, err := ReadFile(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !read.EqualVT(policy) {
		t.Fatalf("allow list changed on disk: %+v", read)
	}

	// Reject each malformed list with its own error.
	cases := map[string]struct {
		ids  []string
		want string
	}{
		"empty id":  {[]string{" "}, "node type id is required"},
		"duplicate": {[]string{"tcp-port", "tcp-port"}, "duplicate device policy node type"},
	}
	for name, tc := range cases {
		err := Validate(&DevicePolicy{NodeTypeId: tc.ids})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: expected %q error, got %v", name, tc.want, err)
		}
	}
}

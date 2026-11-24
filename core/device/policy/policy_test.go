package device_policy

import (
	"os"
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
		LaunchApproval: []*LaunchApproval{
			{SpaceId: "space-a", HostExecution: true, AllowedChildModels: []string{"@fast"}},
		},
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

	// A Space without an entry gets only the isolated launcher.
	if !read.LaunchApprovalFor("space-a").GetHostExecution() || read.LaunchApprovalFor("space-b").GetHostExecution() {
		t.Fatalf("launch approval lookup wrong: %+v", read.GetLaunchApproval())
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

	// Reject each malformed launch approval with its own error.
	approvals := map[string]struct {
		approvals []*LaunchApproval
		want      string
	}{
		"no space":       {[]*LaunchApproval{{HostExecution: true}}, "space id is required"},
		"repeated space": {[]*LaunchApproval{{SpaceId: "a"}, {SpaceId: "a"}}, "duplicate device policy launch approval"},
		"empty model":    {[]*LaunchApproval{{SpaceId: "a", AllowedChildModels: []string{""}}}, "empty child model"},
		"repeated model": {[]*LaunchApproval{{SpaceId: "a", AllowedChildModels: []string{"m", "m"}}}, "repeats child model"},
	}
	for name, tc := range approvals {
		err := Validate(&DevicePolicy{LaunchApproval: tc.approvals})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: expected %q error, got %v", name, tc.want, err)
		}
	}
}

// TestReadFileIgnoresUnknownFields checks that only the allow list affects policy.
func TestReadFileIgnoresUnknownFields(t *testing.T) {
	for _, field := range []string{
		"forge_worker", "forgeWorker",
		"remote_shell", "remoteShell",
		"checkout_root", "checkoutRoot",
	} {
		t.Run(field, func(t *testing.T) {
			// Write a valid policy with an unknown capability field.
			stateRoot := t.TempDir()
			want := &DevicePolicy{Revision: 1, NodeTypeId: []string{"forge-worker"}}
			if err := WriteFile(stateRoot, want); err != nil {
				t.Fatal(err)
			}
			data := `{"revision":"1","nodeTypeId":["forge-worker"],"` + field + `":{"enabled":true}}`
			if err := os.WriteFile(FilePath(stateRoot), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}

			// Read only the declared fields through the production file API.
			got, err := ReadFile(stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if !got.EqualVT(want) {
				t.Fatalf("policy = %+v, want %+v", got, want)
			}
		})
	}
}

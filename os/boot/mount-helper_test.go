//go:build linux

package boot

import "testing"

// TestMountHelperRejectsUnsafeArguments rejects writable and extended mount
// requests before the helper opens a FUSE device or mounts anything.
func TestMountHelperRejectsUnsafeArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-u", "/"},
		{"-o", "rw,fsname=hydrafs,subtype=hydrafs", "--", "/"},
		{"-o", "ro,fsname=hydrafs,subtype=hydrafs,allow_other", "--", "/"},
		{"-o", "ro,ro,subtype=hydrafs", "--", "/"},
	} {
		if err := RunMountHelper(args); err == nil {
			t.Fatalf("accepted unsafe arguments: %q", args)
		}
	}
}

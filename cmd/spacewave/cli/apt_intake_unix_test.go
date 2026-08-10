//go:build unix

package spacewave_cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAptImportDebRejectsUnixNonRegularFiles(t *testing.T) {
	tests := []struct {
		name string
		path func(t *testing.T) string
	}{
		{
			name: "fifo",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "package.fifo")
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
		{
			name: "device",
			path: func(t *testing.T) string { return "/dev/null" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runAptCLI(t, "repo", "package", test.path(t))
			if err == nil || !strings.Contains(err.Error(), "deb package must be a regular file") {
				t.Fatalf("error = %v, want regular-file rejection", err)
			}
		})
	}
}

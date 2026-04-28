package spacewave_loader_controller

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveHelperPath is the IS-1 regression: verifies the helper binary
// name resolution logic appends =.exe= only on Windows when no override is
// supplied, and uses the override verbatim otherwise. Ensures the loader
// finds the right binary adjacent to spacewave.app on each OS.
func TestResolveHelperPath(t *testing.T) {
	cases := []struct {
		name       string
		override   string
		goos       string
		stageFiles []string
		wantFile   string
		wantOK     bool
	}{
		{
			name:       "darwin default finds spacewave-helper",
			goos:       "darwin",
			stageFiles: []string{"spacewave-helper"},
			wantFile:   "spacewave-helper",
			wantOK:     true,
		},
		{
			name:       "linux default finds spacewave-helper",
			goos:       "linux",
			stageFiles: []string{"spacewave-helper"},
			wantFile:   "spacewave-helper",
			wantOK:     true,
		},
		{
			name:       "windows default finds spacewave-helper.exe",
			goos:       "windows",
			stageFiles: []string{"spacewave-helper.exe"},
			wantFile:   "spacewave-helper.exe",
			wantOK:     true,
		},
		{
			name:       "windows default rejects missing .exe suffix",
			goos:       "windows",
			stageFiles: []string{"spacewave-helper"},
			wantOK:     false,
		},
		{
			name:       "darwin default rejects .exe suffix",
			goos:       "darwin",
			stageFiles: []string{"spacewave-helper.exe"},
			wantOK:     false,
		},
		{
			name:       "override used verbatim on darwin",
			override:   "custom-helper",
			goos:       "darwin",
			stageFiles: []string{"custom-helper"},
			wantFile:   "custom-helper",
			wantOK:     true,
		},
		{
			name:       "override used verbatim on windows without auto .exe",
			override:   "custom-helper",
			goos:       "windows",
			stageFiles: []string{"custom-helper"},
			wantFile:   "custom-helper",
			wantOK:     true,
		},
		{
			name:       "override .exe used verbatim on windows",
			override:   "custom-helper.exe",
			goos:       "windows",
			stageFiles: []string{"custom-helper.exe"},
			wantFile:   "custom-helper.exe",
			wantOK:     true,
		},
		{
			name:       "missing binary returns false",
			goos:       "linux",
			stageFiles: nil,
			wantOK:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.stageFiles {
				path := filepath.Join(dir, f)
				if err := os.WriteFile(path, []byte("stub"), 0o755); err != nil {
					t.Fatalf("stage %s: %v", f, err)
				}
			}

			got, ok := resolveHelperPathIn(dir, tc.override, tc.goos)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got path %q)", ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				if got != "" {
					t.Fatalf("path = %q, want empty on miss", got)
				}
				return
			}
			want := filepath.Join(dir, tc.wantFile)
			if got != want {
				t.Fatalf("path = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveHelperPathFromDirsFallback(t *testing.T) {
	pluginDir := t.TempDir()
	hostDir := t.TempDir()
	helperPath := filepath.Join(hostDir, "spacewave-helper")
	if err := os.WriteFile(helperPath, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := resolveHelperPathFromDirs([]string{pluginDir, hostDir}, "", "darwin")
	if !ok {
		t.Fatal("expected helper fallback to host executable dir")
	}
	if got != helperPath {
		t.Fatalf("path = %q, want %q", got, helperPath)
	}
}

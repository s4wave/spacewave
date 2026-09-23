package storagepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageRootEnvVar(t *testing.T) {
	tests := []struct {
		projectID string
		want      string
	}{
		{"spacewave", "SPACEWAVE_DATA_DIR"},
		{"my-project", "MY_PROJECT_DATA_DIR"},
		{"foo bar", "FOOBAR_DATA_DIR"},
		{"weird!@#name", "WEIRDNAME_DATA_DIR"},
	}
	for _, tt := range tests {
		if got := StorageRootEnvVar(tt.projectID); got != tt.want {
			t.Errorf("StorageRootEnvVar(%q) = %q, want %q", tt.projectID, got, tt.want)
		}
	}
}

func TestLogLevelEnvVar(t *testing.T) {
	tests := []struct {
		projectID string
		want      string
	}{
		{"spacewave", "SPACEWAVE_LOG_LEVEL"},
		{"my-project", "MY_PROJECT_LOG_LEVEL"},
		{"foo bar", "FOOBAR_LOG_LEVEL"},
	}
	for _, tt := range tests {
		if got := LogLevelEnvVar(tt.projectID); got != tt.want {
			t.Errorf("LogLevelEnvVar(%q) = %q, want %q", tt.projectID, got, tt.want)
		}
	}
}

func TestLogRetentionDaysEnvVar(t *testing.T) {
	if got := LogRetentionDaysEnvVar("spacewave"); got != "SPACEWAVE_LOG_RETENTION_DAYS" {
		t.Errorf("LogRetentionDaysEnvVar(spacewave) = %q", got)
	}
	if got := LogRetentionDaysEnvVar("my-project"); got != "MY_PROJECT_LOG_RETENTION_DAYS" {
		t.Errorf("LogRetentionDaysEnvVar(my-project) = %q", got)
	}
}

func TestDetermineStorageRootPrefersStatePath(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state")
	dataDir := filepath.Join(t.TempDir(), "data")
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	t.Setenv("SPACEWAVE_DATA_DIR", dataDir)
	if got, err := DetermineStorageRoot("spacewave"); err != nil || got != statePath {
		t.Fatalf("DetermineStorageRoot = %q, %v; want state path %q", got, err, statePath)
	}

	t.Setenv("SPACEWAVE_STATE_PATH", "")
	if got, err := DetermineStorageRoot("spacewave"); err != nil || got != dataDir {
		t.Fatalf("DetermineStorageRoot = %q, %v; want data dir %q", got, err, dataDir)
	}
}

func TestResolveStatePathPublishesAbsoluteRoot(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("SPACEWAVE_STATE_PATH", "")
	t.Setenv("SPACEWAVE_SOCKET_PATH", "")

	root, err := ResolveStatePath("spacewave", "state", "/tmp/sw.sock")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) || filepath.Base(root) != "state" {
		t.Fatalf("root = %q, want absolute path ending in state", root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("state root not created: %v", err)
	}
	if got := os.Getenv("SPACEWAVE_STATE_PATH"); got != root {
		t.Fatalf("SPACEWAVE_STATE_PATH = %q, want %q", got, root)
	}
	if got := os.Getenv("SPACEWAVE_SOCKET_PATH"); got != "/tmp/sw.sock" {
		t.Fatalf("SPACEWAVE_SOCKET_PATH = %q", got)
	}
	if got, _ := DetermineStorageRoot("spacewave"); got != root {
		t.Fatalf("DetermineStorageRoot = %q, want %q", got, root)
	}
}

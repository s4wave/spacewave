//go:build !js

package resource_listener

import (
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/util/gitroot"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	"github.com/sirupsen/logrus"
)

func TestDetermineSocketPathUsesDataRootOverride(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "qa")
	t.Setenv("SPACEWAVE_DATA_DIR", dataRoot)

	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataRoot, "spacewave.sock")
	if got != want {
		t.Fatalf("socket path: got %q, want %q", got, want)
	}
}

func TestDetermineSocketPathUsesPlatformConfigRootWithoutOverride(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("SPACEWAVE_DATA_DIR", "")

	configRoot, err := storagepath.DetermineConfigDir("spacewave")
	if err != nil {
		t.Fatal(err)
	}
	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(configRoot, "spacewave.sock")
	if got != want {
		t.Fatalf("socket path: got %q, want %q", got, want)
	}
}

func TestDetermineSocketPathEmptyConfigDisablesListener(t *testing.T) {
	got, err := (&Config{}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("empty config enabled listener at %q", got)
	}
}

func TestExplicitSocketPathsTakePrecedenceAndResolve(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataRoot := filepath.Join(t.TempDir(), "storage")
	t.Setenv("SPACEWAVE_DATA_DIR", dataRoot)

	gitRoot, err := gitroot.FindRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	absolutePath := filepath.Join(t.TempDir(), "absolute.sock")
	tests := []struct {
		name     string
		explicit string
		want     string
	}{
		{
			name:     "absolute",
			explicit: absolutePath,
			want:     absolutePath,
		},
		{
			name:     "git root relative",
			explicit: "git:listener-test.sock",
			want:     filepath.Join(gitRoot, "listener-test.sock"),
		},
		{
			name:     "home relative",
			explicit: "~/listener-test.sock",
			want:     filepath.Join(home, "listener-test.sock"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configured, err := (&Config{
				ListenerSocketPath: test.explicit,
				StorageProjectId:   "spacewave",
			}).DetermineSocketPath()
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolveSocketPath(logrus.NewEntry(logrus.New()), configured)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("resolved socket path: got %q, want %q", got, test.want)
			}
		})
	}
}

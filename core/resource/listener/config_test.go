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
	// Configure a data root that supplies the listener socket directory.
	dataRoot := filepath.Join(t.TempDir(), "qa")
	t.Setenv("SPACEWAVE_DATA_DIR", dataRoot)

	// Resolve the listener socket from the configured data root.
	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the listener socket belongs to the overridden data root.
	want := filepath.Join(dataRoot, "spacewave.sock")
	if got != want {
		t.Fatalf("socket path: got %q, want %q", got, want)
	}
}

func TestDetermineSocketPathUsesPlatformConfigRootWithoutOverride(t *testing.T) {
	// Isolate the platform home and remove the listener data override.
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("SPACEWAVE_DATA_DIR", "")

	// Determine the platform configuration directory for the listener.
	configRoot, err := storagepath.DetermineConfigDir("spacewave")
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the listener socket with the platform configuration root.
	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the listener socket uses the platform configuration directory.
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
	// Configure home and storage roots that explicit socket paths override.
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataRoot := filepath.Join(t.TempDir(), "storage")
	t.Setenv("SPACEWAVE_DATA_DIR", dataRoot)

	// Prepare explicit socket paths rooted in Git, home, and an absolute directory.
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

	// Check each explicit socket path against its resolved directory.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Resolve the configured explicit socket before expanding its path prefix.
			configured, err := (&Config{
				ListenerSocketPath: test.explicit,
				StorageProjectId:   "spacewave",
			}).DetermineSocketPath()
			if err != nil {
				t.Fatal(err)
			}

			// Expand the socket path using the requested directory prefix.
			got, err := resolveSocketPath(logrus.NewEntry(logrus.New()), configured)
			if err != nil {
				t.Fatal(err)
			}

			// Check that the explicit socket path takes precedence over storage defaults.
			if got != test.want {
				t.Fatalf("resolved socket path: got %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetermineSocketPathUsesStatePath(t *testing.T) {
	// Configure a state path independently of home and listener overrides.
	home := filepath.Join(t.TempDir(), "home")
	statePath := filepath.Join(t.TempDir(), "state")
	t.Setenv("HOME", home)
	t.Setenv("SPACEWAVE_DATA_DIR", "")
	t.Setenv(storagepath.SocketPathEnvVar("spacewave"), "")
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)

	// Resolve the listener socket from the configured state path.
	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the listener socket uses the state directory.
	want := filepath.Join(statePath, "spacewave.sock")
	if got != want {
		t.Fatalf("socket path: got %q, want %q", got, want)
	}
}

func TestDetermineSocketPathUsesExplicitSocketPath(t *testing.T) {
	// Configure an explicit listener socket alongside the state directory.
	home := filepath.Join(t.TempDir(), "home")
	statePath := filepath.Join(t.TempDir(), "state")
	explicit := filepath.Join(t.TempDir(), "explicit.sock")
	t.Setenv("HOME", home)
	t.Setenv("SPACEWAVE_DATA_DIR", "")
	t.Setenv("SPACEWAVE_STATE_PATH", statePath)
	t.Setenv("SPACEWAVE_SOCKET_PATH", explicit)

	// Resolve the listener socket from the environment override.
	got, err := (&Config{StorageProjectId: "spacewave"}).DetermineSocketPath()
	if err != nil {
		t.Fatal(err)
	}

	// Check that the explicit listener socket overrides the state directory.
	if got != explicit {
		t.Fatalf("socket path: got %q, want %q", got, explicit)
	}
}

func TestSocketPathManaged(t *testing.T) {
	// Check managed socket parents for state-derived and configured paths.
	t.Setenv(storagepath.SocketPathEnvVar("spacewave"), "")
	if !(&Config{StorageProjectId: "spacewave"}).SocketPathManaged() {
		t.Fatal("state-derived socket path must use its managed parent")
	}
	if (&Config{
		ListenerSocketPath: "/tmp/explicit.sock",
		StorageProjectId:   "spacewave",
	}).SocketPathManaged() {
		t.Fatal("configured socket path parent must not be managed")
	}

	// Check that an environment socket override keeps its parent unmanaged.
	t.Setenv(storagepath.SocketPathEnvVar("spacewave"), "/tmp/environment.sock")
	if (&Config{StorageProjectId: "spacewave"}).SocketPathManaged() {
		t.Fatal("environment socket path parent must not be managed")
	}
}

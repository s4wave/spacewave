package spacewave_launcher_controller

import (
	"testing"
)

func TestLauncherDetectsMacOSMismatch(t *testing.T) {
	tests := []struct {
		name       string
		execPath   string
		wantBundle bool
		wantRoot   string
	}{
		{
			name:       "macos app bundle",
			execPath:   "/Applications/Spacewave.app/Contents/MacOS/spacewave",
			wantBundle: true,
			wantRoot:   "/Applications/Spacewave.app",
		},
		{
			name:       "macos app bundle in user dir",
			execPath:   "/Users/foo/Applications/Spacewave.app/Contents/MacOS/spacewave",
			wantBundle: true,
			wantRoot:   "/Users/foo/Applications/Spacewave.app",
		},
		{
			name:       "linux binary",
			execPath:   "/usr/local/bin/spacewave",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "windows binary",
			execPath:   "C:\\Program Files\\Spacewave\\spacewave.exe",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "macos binary outside app bundle",
			execPath:   "/usr/local/bin/spacewave",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "partial app structure missing MacOS",
			execPath:   "/Applications/Spacewave.app/Contents/spacewave",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "partial app structure missing Contents",
			execPath:   "/Applications/Spacewave.app/MacOS/spacewave",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "not ending in .app",
			execPath:   "/Applications/Spacewave/Contents/MacOS/spacewave",
			wantBundle: false,
			wantRoot:   "",
		},
		{
			name:       "nested app bundle",
			execPath:   "/tmp/build/MyApp.app/Contents/MacOS/myapp",
			wantBundle: true,
			wantRoot:   "/tmp/build/MyApp.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isBundle, bundleRoot := detectAppBundle(tt.execPath)
			if isBundle != tt.wantBundle {
				t.Errorf("detectAppBundle(%q) isBundle = %v, want %v", tt.execPath, isBundle, tt.wantBundle)
			}
			if bundleRoot != tt.wantRoot {
				t.Errorf("detectAppBundle(%q) bundleRoot = %q, want %q", tt.execPath, bundleRoot, tt.wantRoot)
			}
		})
	}
}

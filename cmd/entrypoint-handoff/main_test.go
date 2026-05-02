//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsBuilderImage(t *testing.T) {
	tests := []struct {
		name      string
		hostGOOS  string
		platforms []string
		want      bool
	}{
		{
			name:      "darwin only never needs docker builder",
			hostGOOS:  "darwin",
			platforms: []string{"darwin-amd64", "darwin-arm64"},
			want:      false,
		},
		{
			name:      "linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"linux-amd64"},
			want:      true,
		},
		{
			name:      "windows on linux needs docker builder",
			hostGOOS:  "linux",
			platforms: []string{"windows-amd64"},
			want:      true,
		},
		{
			name:      "windows on windows builds natively",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "windows-arm64"},
			want:      false,
		},
		{
			name:      "windows and linux on windows still need docker builder for linux",
			hostGOOS:  "windows",
			platforms: []string{"windows-amd64", "linux-amd64"},
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := needsBuilderImage(test.hostGOOS, test.platforms)
			if got != test.want {
				t.Fatalf("needsBuilderImage(%q, %#v) = %v, want %v", test.hostGOOS, test.platforms, got, test.want)
			}
		})
	}
}

func TestValidateRemoteHandoffManifestRejectsStaleHash(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, ".bldr", "build", "js", "spacewave-app", "dist", "app.js")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := remoteHandoffIdentity{
		GitSHA:             "abc123",
		ReleaseEnv:         "staging",
		ReactDev:           true,
		RemoteTargetNames:  remoteHandoffTargets,
		RemoteFileMetadata: files,
	}
	if err := os.WriteFile(filepath.Join(dir, "remote-manifest.json"), marshalRemoteHandoffManifest(identity), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteHandoffManifest(dir, identity); err != nil {
		t.Fatalf("validateRemoteHandoffManifest valid = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteHandoffManifest(dir, identity); err == nil {
		t.Fatal("validateRemoteHandoffManifest accepted stale file hash")
	}
}

func TestValidateRemoteHandoffManifestRejectsReactDevMismatch(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, ".bldr", "build", "js", "spacewave-web", "dist", "web.js")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("web"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := hashTree(root)
	if err != nil {
		t.Fatal(err)
	}
	identity := remoteHandoffIdentity{
		GitSHA:             "abc123",
		ReleaseEnv:         "staging",
		ReactDev:           true,
		RemoteTargetNames:  remoteHandoffTargets,
		RemoteFileMetadata: files,
	}
	if err := os.WriteFile(filepath.Join(dir, "remote-manifest.json"), marshalRemoteHandoffManifest(identity), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := identity
	expected.ReactDev = false
	if err := validateRemoteHandoffManifest(dir, expected); err == nil {
		t.Fatal("validateRemoteHandoffManifest accepted react_dev mismatch")
	}
}

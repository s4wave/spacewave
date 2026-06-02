package cli_entrypoint

import (
	"bytes"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestStandaloneVersionCommandReportsUnmanagedIdentity(t *testing.T) {
	var buf bytes.Buffer
	app := cli.NewApp()
	app.Writer = &buf
	app.Commands = []*cli.Command{newStandaloneVersionCommand("spacewave")}
	if err := app.Run([]string{"spacewave", "version", "--json"}); err != nil {
		t.Fatal(err)
	}

	var got standaloneVersionIdentity
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProjectID != "spacewave" {
		t.Fatalf("project = %q, want spacewave", got.ProjectID)
	}
	if got.EntrypointRole != "standalone" {
		t.Fatalf("entrypoint role = %q, want standalone", got.EntrypointRole)
	}
	wantPlatform := "desktop/" + runtime.GOOS + "/" + runtime.GOARCH
	if got.PlatformID != wantPlatform {
		t.Fatalf("platform = %q, want %q", got.PlatformID, wantPlatform)
	}
	if got.Manifest.ManifestID != "" || got.Manifest.Rev != 0 {
		t.Fatalf("manifest identity = %#v, want empty", got.Manifest)
	}
}

package cli_entrypoint

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
)

func TestStandaloneVersionCommandReportsUnmanagedIdentity(t *testing.T) {
	var buf bytes.Buffer
	app := cli.NewApp()
	app.Writer = &buf
	app.Commands = []*cli.Command{newStandaloneVersionCommand("spacewave")}
	if err := app.Run([]string{"spacewave", "version", "--json"}); err != nil {
		t.Fatal(err)
	}

	var parser fastjson.Parser
	got, err := parser.ParseBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if projectID := string(got.GetStringBytes("projectId")); projectID != "spacewave" {
		t.Fatalf("project = %q, want spacewave", projectID)
	}
	if role := string(got.GetStringBytes("entrypointRole")); role != "standalone" {
		t.Fatalf("entrypoint role = %q, want standalone", role)
	}
	wantPlatform := "desktop/" + runtime.GOOS + "/" + runtime.GOARCH
	if platformID := string(got.GetStringBytes("platformId")); platformID != wantPlatform {
		t.Fatalf("platform = %q, want %q", platformID, wantPlatform)
	}
	if manifestID := string(got.GetStringBytes("manifest", "manifestId")); manifestID != "" {
		t.Fatalf("manifest ID = %q, want empty", manifestID)
	}
	if manifestRev := got.GetUint64("manifest", "rev"); manifestRev != 0 {
		t.Fatalf("manifest rev = %d, want 0", manifestRev)
	}
}

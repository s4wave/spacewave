package dist_entrypoint

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/aperturerobotics/cli"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestDistVersionCommandReportsManagedCLIIdentity(t *testing.T) {
	meta := bldr_dist.NewDistEntrypointMeta(
		"spacewave",
		"desktop/darwin/arm64",
		[]string{"spacewave-launcher", "spacewave-core"},
		&bucket.ObjectRef{},
		"dist",
		bldr_dist.EntrypointRoleCLI,
		"stable",
		"spacewave-dist",
		224,
	)
	var buf bytes.Buffer
	app := cli.NewApp()
	app.Writer = &buf
	app.Commands = []*cli.Command{newDistVersionCommand(meta)}
	if err := app.Run([]string{"spacewave", "version", "--json"}); err != nil {
		t.Fatal(err)
	}

	var got distVersionIdentity
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.EntrypointRole != bldr_dist.EntrypointRoleCLI {
		t.Fatalf("entrypoint role = %q, want cli", got.EntrypointRole)
	}
	if got.ChannelKey != "stable" {
		t.Fatalf("channel = %q, want stable", got.ChannelKey)
	}
	if got.PlatformID != "desktop/darwin/arm64" {
		t.Fatalf("platform = %q, want desktop/darwin/arm64", got.PlatformID)
	}
	if got.Manifest.ManifestID != "spacewave-dist" || got.Manifest.Rev != 224 {
		t.Fatalf("manifest identity = %#v, want spacewave-dist rev 224", got.Manifest)
	}
}

package dist_entrypoint

import (
	"bytes"
	"testing"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
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

	var parser fastjson.Parser
	got, err := parser.ParseBytes(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if role := string(got.GetStringBytes("entrypointRole")); role != bldr_dist.EntrypointRoleCLI {
		t.Fatalf("entrypoint role = %q, want cli", role)
	}
	if channel := string(got.GetStringBytes("channelKey")); channel != "stable" {
		t.Fatalf("channel = %q, want stable", channel)
	}
	if platform := string(got.GetStringBytes("platformId")); platform != "desktop/darwin/arm64" {
		t.Fatalf("platform = %q, want desktop/darwin/arm64", platform)
	}
	if manifestID, rev := string(got.GetStringBytes("manifest", "manifestId")), got.GetUint64("manifest", "rev"); manifestID != "spacewave-dist" || rev != 224 {
		t.Fatalf("manifest identity = %s rev %d, want spacewave-dist rev 224", manifestID, rev)
	}
}

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
	if got.GetStringBytes("entrypointRole") == nil || string(got.GetStringBytes("entrypointRole")) != bldr_dist.EntrypointRoleCLI {
		t.Fatalf("entrypoint role = %q, want cli", got.GetStringBytes("entrypointRole"))
	}
	if string(got.GetStringBytes("channelKey")) != "stable" {
		t.Fatalf("channel = %q, want stable", got.GetStringBytes("channelKey"))
	}
	if string(got.GetStringBytes("platformId")) != "desktop/darwin/arm64" {
		t.Fatalf("platform = %q, want desktop/darwin/arm64", got.GetStringBytes("platformId"))
	}
	if got.Get("manifest") == nil {
		t.Fatal("manifest identity missing")
	}
	if string(got.GetStringBytes("manifest", "manifestId")) != "spacewave-dist" || got.GetUint("manifest", "rev") != 224 {
		t.Fatalf("manifest identity = %s, want spacewave-dist rev 224", got.Get("manifest"))
	}
}

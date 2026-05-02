//go:build !js

package main

import (
	"context"
	"io"
	"os"

	appcli "github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/cmd/entrypoint-handoff/handoff"
)

func main() {
	args := &handoff.Args{}
	args.FillDefaults()
	app := appcli.NewApp()
	app.Name = "entrypoint-handoff"
	app.Usage = "build entrypoint release handoff artifacts"
	app.Flags = []appcli.Flag{
		&appcli.StringFlag{Name: "version", Usage: "release version", Destination: &args.Version},
		&appcli.StringFlag{Name: "platforms", Usage: "comma-separated target platforms", Destination: &args.PlatformsCSV},
		&appcli.StringFlag{Name: "out-dir", Usage: "path to the staged handoff output dir", Destination: &args.OutDir},
		&appcli.BoolFlag{Name: "react-dev", Usage: "build browser entrypoint in dev mode", Destination: &args.ReactDev},
		&appcli.BoolFlag{Name: "skip-notarize", Usage: "skip Apple notarization during packaging", Destination: &args.SkipNotarize},
		&appcli.BoolFlag{Name: "include-browser", Usage: "include browser staging tree and static-manifest.ts", Destination: &args.IncludeBrowser},
		&appcli.BoolFlag{Name: "browser-only", Usage: "build only browser staging tree and static-manifest.ts", Destination: &args.BrowserOnly},
		&appcli.BoolFlag{Name: "skip-build", Usage: "skip helper and entrypoint builds and package existing artifacts", Destination: &args.SkipBuild},
		&appcli.BoolFlag{Name: "skip-package", Usage: "skip installer packaging", Destination: &args.SkipPackage},
		&appcli.BoolFlag{Name: "stage-build-inputs", Usage: "stage raw dist/helper/icon inputs into out-dir", Destination: &args.StageBuildInputs},
		&appcli.BoolFlag{Name: "remote-only", Usage: "build only shared remote entrypoint outputs", Destination: &args.RemoteOnly},
		&appcli.StringFlag{Name: "remote-handoff-dir", Usage: "validated shared remote entrypoint handoff input dir", Destination: &args.RemoteHandoffDir},
		&appcli.BoolFlag{Name: "manifest-pack-produce", Usage: "produce one manifest-pack artifact and exit", Destination: &args.ManifestPackProduce},
		&appcli.StringFlag{Name: "manifest-pack-import-dirs", Usage: "comma-separated manifest-pack artifact dirs to import before building", Destination: &args.ManifestPackImportDirsCSV},
		&appcli.StringFlag{Name: "manifest-id", Usage: "manifest-pack manifest id", Destination: &args.ManifestID},
		&appcli.StringFlag{Name: "manifest-platform", Usage: "manifest-pack platform id", Destination: &args.ManifestPlatformID},
		&appcli.StringFlag{Name: "manifest-object-key", Usage: "manifest-pack bundle object key", Destination: &args.ManifestObjectKey},
		&appcli.StringFlag{Name: "manifest-link-object-keys", Usage: "comma-separated manifest-pack link object keys", Destination: &args.ManifestLinkObjectKeysCSV},
		&appcli.StringFlag{Name: "manifest-producer-target", Usage: "manifest-pack producer target name", Destination: &args.ManifestProducerTarget},
		&appcli.StringFlag{Name: "manifest-cache-schema", Usage: "manifest-pack cache schema", Destination: &args.ManifestCacheSchema},
	}
	app.Action = func(ctx *appcli.Context) error {
		return handoff.Run(context.Background(), args)
	}
	if err := app.Run(os.Args); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

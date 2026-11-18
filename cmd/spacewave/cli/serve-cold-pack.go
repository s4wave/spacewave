//go:build !js

package spacewave_cli

import (
	"github.com/aperturerobotics/cli"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// serveColdPackFlags configures one optional, host-local cold-pack tier.
func serveColdPackFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "cold-pack-store", Usage: "placed block store ID, p/<provider>/<account>/blk/<id>"},
		&cli.StringFlag{Name: "cold-pack-backend", Usage: "S3 storage backend ID of the placed block store"},
		&cli.StringFlag{Name: "cold-pack-prefix", Usage: "cold object prefix, outside the hot store's prefix in the same S3 bucket"},
		&cli.StringFlag{Name: "cold-pack-manifest", Usage: "path to the local cold pack manifest"},
		&cli.StringFlag{Name: "cold-pack-promoted-log", Usage: "local promoted log path; omit to disable persistent promotion"},
		&cli.IntFlag{Name: "cold-pack-promote-cap", Usage: "maximum promoted keys; zero disables persistent promotion"},
		&cli.IntFlag{Name: "cold-pack-tail-cache-bytes", Usage: "cold index-tail cache budget in bytes; zero uses the default"},
		&cli.IntFlag{Name: "cold-pack-negative-cache-keys", Usage: "cold negative-cache key limit; zero uses the default"},
	}
}

// configureServeColdPacks installs the optional placement before opening the
// CLI bus. A nil release means the command did not configure cold packs.
func configureServeColdPacks(c *cli.Context) (func(), error) {
	// Use the same flag list to detect partial configuration.
	var configured bool
	for _, flag := range serveColdPackFlags() {
		if c.IsSet(flag.Names()[0]) {
			configured = true
			break
		}
	}
	if !configured {
		return nil, nil
	}

	// The local provider resolves the placement's existing bucket and credentials.
	return provider_local.ConfigureColdPacks(provider_local.ColdPackPlacementConfig{
		BlockStoreLocalID: c.String("cold-pack-store"),
		BackendID:         c.String("cold-pack-backend"),
		ColdPackOptions: block_store_s3.ColdPackOptions{
			Prefix:            c.String("cold-pack-prefix"),
			ManifestFile:      c.String("cold-pack-manifest"),
			PromotedLog:       c.String("cold-pack-promoted-log"),
			PromoteCap:        c.Int("cold-pack-promote-cap"),
			TailCacheBytes:    c.Int("cold-pack-tail-cache-bytes"),
			NegativeCacheKeys: c.Int("cold-pack-negative-cache-keys"),
		},
	})
}

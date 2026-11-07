//go:build !js

package main

import (
	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/cmd/spacewave-store-bench/storebench"
	"github.com/sirupsen/logrus"
)

// StoreArgs locate the World in the store and size its caches.
type StoreArgs struct {
	// Store is the store location and cache sizes.
	Store storebench.StoreConfig
	// Verbose logs at debug level.
	Verbose bool
}

// BuildFlags binds the store flags. A zero size keeps the server's, so the
// defaults run the store as a server does.
func (a *StoreArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "store-dir", Usage: "storage root directory of the stopped store copy", Destination: &a.Store.Dir, Required: true},
		&cli.StringFlag{Name: "peer-volume", Usage: "storage volume whose peer runs the local provider", Destination: &a.Store.PeerVolumeID, Value: "store-bench-peer"},
		&cli.StringFlag{Name: "account", Usage: "local provider account that holds the World", Destination: &a.Store.AccountID, Value: "store-bench"},
		&cli.StringFlag{Name: "shared-object", Usage: "the World's SharedObject in the account", Destination: &a.Store.SharedObjectID, Value: "store-bench-world"},
		&cli.StringFlag{Name: "engine-id", Usage: "World engine ID", Destination: &a.Store.EngineID, Value: "store-bench"},
		&cli.Int64Flag{Name: "cache-bytes", Usage: "index page cache of each s4db volume; zero keeps the server's", Destination: &a.Store.CacheBytes},
		&cli.Int64Flag{Name: "checkpoint-max-bytes", Usage: "overlay size that starts an s4db checkpoint; zero keeps the server's", Destination: &a.Store.CheckpointMaxBytes},
		&cli.Int64Flag{Name: "block-cache-bytes", Usage: "decoded block cache budget; zero keeps the default", Destination: &a.Store.BlockCacheBytes},
		&cli.StringFlag{Name: "pack-dir", Usage: "directory of the World store's packahead log; empty keeps every write in the volume", Destination: &a.Store.PackDir},
		&cli.BoolFlag{Name: "pack-divert", Usage: "append the World store's new values to its packahead log", Destination: &a.Store.PackDivert},
		&cli.BoolFlag{Name: "verbose", Usage: "log at debug level", Destination: &a.Verbose},
	}
}

// logger returns the command's logger.
func (a *StoreArgs) logger() *logrus.Entry {
	log := logrus.New()
	if a.Verbose {
		log.SetLevel(logrus.DebugLevel)
	}
	return logrus.NewEntry(log)
}

package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bldr/banner"
	dist_entrypoint "github.com/aperturerobotics/bldr/dist/entrypoint"
	fcolor "github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

// This is a rough prototype of the entrypoint executable we will ship in
// production. The goal is to ship a single statically-linked executable that
// then downloads and loads other resources.

var AppID = "bldr-dist" // TODO

var LogLevel = logrus.DebugLevel // TODO

func main() {
	log := logrus.New()
	log.SetLevel(LogLevel)
	le := logrus.NewEntry(log)

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	storageRoot, err := DetermineStorageRoot(AppID)
	if err != nil {
		le.WithError(err).Warn("unable to determine storage root, using current dir")
		storageRoot = "./state"
	}
	distBus, err := dist_entrypoint.BuildDistBus(ctx, le, AppID, storageRoot)
	if err != nil {
		le.WithError(err).Fatal("unable to initialize application")
	}
	le.Info("host is ready")
	// TODO
	_ = distBus

	writeBanner()
	// <-time.After(time.Second)
	distBus.Release()
}

// writeBanner writes the banner in red to os.stderr.
func writeBanner() {
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(os.Stderr, banner.FormatBanner()+"\n")
}

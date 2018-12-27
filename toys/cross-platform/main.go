package main

import (
	"context"

	"github.com/aperturerobotics/hydra/core"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		panic(err)
	}

	// TODO: add storage depending on if we are in js or not.
	av, ref, err := addStorageVolume(ctx, le, b, sr)
	if err != nil {
		panic(err)
	}
	defer ref.Release()

	le.Info("storage volume resolved")
	_ = av
	// TODO: store something
	// TODO: retrieve it
	<-ctx.Done()
}

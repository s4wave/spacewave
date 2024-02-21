//go:build js
// +build js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint the web.
func Main(distMetaB58 string, logLevel logrus.Level, assetsFS fs.FS) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		DisableTimestamp: false,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	// There is no os.Interrupt on js.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	if err := func() error {
		distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
		if err != nil {
			return err
		}

		err = Run(ctx, le, distMeta, assetsFS)
		if err != context.Canceled {
			return err
		}
		return nil
	}(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

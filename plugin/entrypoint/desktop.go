//go:build !js
// +build !js

package plugin_entrypoint

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bldr/util/pipesock"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the entrypoint version
var Version = semver.MustParse("0.0.1")

// Main runs the default main entrypoint for a program.
func Main(addFactoryFuncs []AddFactoryFunc, configSetFuncs []BuildConfigSetFunc) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	if err := Run(ctx, le, addFactoryFuncs, configSetFuncs); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// Run runs the plugin entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// construct pipe socket
	conn, err := pipesock.DialPipeListener(ctx, le, wd, "plugin")
	if err != nil {
		return err
	}

	// construct mplex
	muxedConn, err := srpc.NewMuxedConn(conn, false, nil)
	if err != nil {
		return err
	}
	defer muxedConn.Close()

	return ExecutePlugin(ctx, le, addFactoryFuncs, configSetFuncs, muxedConn)
}

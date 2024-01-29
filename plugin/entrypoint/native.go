//go:build !js
// +build !js

package plugin_entrypoint

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/bldr/util/pipesock"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the entrypoint version
var Version = semver.MustParse("0.0.1")

// Main runs the default main entrypoint for a plugin.
func Main(
	pluginStartInfoB58,
	pluginMetaB58 string,
	logLevel logrus.Level,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	// Handle stop signals by canceling the context.
	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	if err := func() error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		pluginMeta, err := bldr_plugin.UnmarshalPluginMetaB58(pluginMetaB58)
		if err != nil {
			return err
		}

		// If the environment variable with the startup info was empty, try to load
		// it from the .plugin-start-info file in the current working dir.
		if pluginStartInfoB58 == "" {
			startInfoBin, err := os.ReadFile(filepath.Join(wd, ".plugin-start-info"))
			if err != nil {
				if err == os.ErrNotExist {
					return errors.New("start info not found")
				}
				return errors.Wrap(err, "load start info")
			}
			pluginStartInfoB58 = string(startInfoBin)
		}

		pluginStartInfo, err := bldr_plugin.UnmarshalPluginStartInfoB58(pluginStartInfoB58)
		if err != nil {
			return err
		}

		err = Run(ctx, le, pluginStartInfo, pluginMeta, addFactoryFuncs, configSetFuncs)
		if err != context.Canceled {
			return err
		}
		return nil
	}(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// Run runs the plugin entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	pluginStartInfo *bldr_plugin.PluginStartInfo,
	pluginMeta *bldr_plugin.PluginMeta,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := pluginStartInfo.Validate(); err != nil {
		return err
	}

	// construct pipe socket
	instanceID := pluginStartInfo.GetInstanceId()
	conn, err := pipesock.DialPipeListener(ctx, le, wd, instanceID)
	if err != nil {
		return err
	}

	// yamux config
	yamuxConf := srpc.NewYamuxConfig()
	yamuxConf.EnableKeepAlive = false

	// construct mplex
	muxedConn, err := srpc.NewMuxedConn(conn, false, yamuxConf)
	if err != nil {
		return err
	}
	defer muxedConn.Close()

	return ExecutePlugin(ctx, le, pluginMeta, addFactoryFuncs, configSetFuncs, muxedConn)
}

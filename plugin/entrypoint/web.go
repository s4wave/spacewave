//go:build js
// +build js

package plugin_entrypoint

import (
	"context"
	"os"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/blang/semver"
	"github.com/libp2p/go-libp2p/core/network"
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

	// There is no os.Interrupt on js.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	if err := func() error {
		pluginStartInfo, pluginMeta, err := UnmarshalPluginStartInfo(pluginStartInfoB58, pluginMetaB58)
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
	if err := pluginStartInfo.Validate(); err != nil {
		return err
	}

	// TODO: construct MessagePort socket
	instanceID := pluginStartInfo.GetInstanceId()
	_ = instanceID
	// conn, err := pipesock.DialPipeListener(ctx, le, wd, instanceID)
	// if err != nil {
	//	return err
	//}
	var muxedConn network.MuxedConn

	return ExecutePlugin(ctx, le, pluginMeta, addFactoryFuncs, configSetFuncs, muxedConn)
}

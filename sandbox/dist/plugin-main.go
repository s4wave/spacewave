package main

import (
	"context"
	"os"

	"github.com/aperturerobotics/bifrost/util/rwc"
	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

func main() {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	os.Stderr.WriteString("hello from plugin\n")

	// construct mplex
	ctx := context.Background()
	inOutRwc := rwc.NewReadWriteCloser(os.Stdin, os.Stdout)
	muxedConn, err := srpc.NewMuxedConn(inOutRwc, true)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	defer muxedConn.Close()

	// construct plugin host client
	client := srpc.NewClientWithMuxedConn(muxedConn)
	pluginHostClient := plugin.NewSRPCPluginHostClient(client)

	// load demo-plugin
	go func() {
		_, err := pluginHostClient.LoadPlugin(ctx, &plugin.LoadPluginRequest{
			PluginId: "sandbox-demo-plugin",
		})
		if err != nil && err != context.Canceled {
			os.Stderr.WriteString(err.Error() + "\n")
		}
	}()

	// listen for incoming requests, exit once closed
	mux := srpc.NewMux()
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	_ = sr
	_ = plugin.SRPCRegisterPluginFetch(mux, NewPluginFetch(le, b))

	srv := srpc.NewServer(mux)
	if err := srv.AcceptMuxedConn(ctx, muxedConn); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

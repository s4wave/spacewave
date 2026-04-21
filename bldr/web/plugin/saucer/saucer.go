package saucer

import (
	"context"
	"encoding/base64"
	"net"
	"os"
	oexec "os/exec"

	bldr_saucer "github.com/aperturerobotics/bldr-saucer"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/exec"
	"github.com/aperturerobotics/util/pipesock"
	singleton_muxed_conn "github.com/s4wave/spacewave/bldr/util/singleton-muxed-conn"
	random_id "github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

// Saucer is a running instance of the Saucer webview.
type Saucer struct {
	ctx context.Context
	cmd *oexec.Cmd

	runtimeUuid string
	saucerPath  string

	// conn is the single yamux connection (C++ -> Go).
	// C++ opens streams to Go for all scheme request forwarding.
	conn     *singleton_muxed_conn.SingletonMuxedConn
	listener net.Listener
}

// RunSaucer listens on the IPC pipe and starts the Saucer sub-process.
func RunSaucer(
	ctx context.Context,
	le *logrus.Entry,
	saucerPath,
	workdirPath,
	runtimeUuid string,
	extraSaucerFlags []string,
	saucerInit *SaucerInit,
) (*Saucer, error) {
	pipeRoot := workdirPath

	// Create yamux pipe.
	// C++ acts as the client (opens streams to us).
	// We act as the server (accept streams from C++).
	le.Debug("listening on yamux socket")
	pipeListener, err := pipesock.BuildPipeListener(le, pipeRoot, runtimeUuid)
	if err != nil {
		return nil, err
	}

	// C++ acts as the client (outbound=true on C++ side).
	// We act as the server (outbound=false).
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, false)
	go smc.AcceptPump(pipeListener)

	_ = os.Chmod(saucerPath, 0o755) // try to chmod

	var saucerArgs []string
	saucerArgs = append(saucerArgs, extraSaucerFlags...)

	cmd := exec.NewCmd(ctx, saucerPath, saucerArgs...)
	cmd.Dir = workdirPath
	cmd.Env = append(cmd.Env, "BLDR_RUNTIME_ID="+runtimeUuid)
	cmd.Env = append(cmd.Env, "BLDR_WEB_DOCUMENT_ID="+random_id.RandomIdentifier(8))

	// Pass SaucerInit as base64-encoded protobuf via env var.
	if saucerInit != nil {
		initMsg := &bldr_saucer.SaucerInit{
			DevTools:      saucerInit.DevTools,
			ExternalLinks: bldr_saucer.ExternalLinks(saucerInit.ExternalLinks),
			AppName:       saucerInit.AppName,
			WindowTitle:   saucerInit.WindowTitle,
			WindowWidth:   saucerInit.WindowWidth,
			WindowHeight:  saucerInit.WindowHeight,
		}
		initBytes, err := initMsg.MarshalVT()
		if err != nil {
			_ = pipeListener.Close()
			_ = smc.CloseWithErr(err)
			return nil, err
		}
		cmd.Env = append(cmd.Env, "BLDR_SAUCER_INIT="+base64.StdEncoding.EncodeToString(initBytes))
	}

	cmd.Stdout = le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = le.WriterLevel(logrus.DebugLevel)

	le.Debugf("starting saucer: %s", cmd.String())
	err = cmd.Start()
	if err != nil {
		_ = pipeListener.Close()
		_ = smc.CloseWithErr(err)
		return nil, err
	}

	// Watch for context cancellation and kill the process.
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	return &Saucer{
		ctx: ctx,
		cmd: cmd,

		runtimeUuid: runtimeUuid,
		saucerPath:  saucerPath,

		conn:     smc,
		listener: pipeListener,
	}, nil
}

// GetMuxedConn returns the main muxed conn with the C++ process.
func (s *Saucer) GetMuxedConn() srpc.MuxedConn {
	return s.conn
}

// GetCmd returns the running Saucer command.
func (s *Saucer) GetCmd() *oexec.Cmd {
	return s.cmd
}

// Close shuts down the saucer instance.
func (s *Saucer) Close() {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

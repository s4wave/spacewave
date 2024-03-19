package electron

import (
	"context"
	"net"
	"os"
	oexec "os/exec"

	"github.com/aperturerobotics/bldr/util/pipesock"
	singleton_muxed_conn "github.com/aperturerobotics/bldr/util/singleton-muxed-conn"
	"github.com/aperturerobotics/util/exec"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/sirupsen/logrus"
)

// Electron is a running instance of Electron.
type Electron struct {
	ctx context.Context
	cmd *oexec.Cmd

	runtimeUuid  string
	electronPath string
	rendererPath string

	ipc      *singleton_muxed_conn.SingletonMuxedConn
	listener net.Listener
}

// RunElectron listens on the IPC pipe and starts Electron sub-process.
func RunElectron(
	ctx context.Context,
	le *logrus.Entry,
	electronPath,
	workdirPath,
	rendererPath,
	runtimeUuid string,
	extraElectronFlags []string,
) (*Electron, error) {
	le.Debug("listening on ipc socket")
	pipeRoot := workdirPath
	pipeListener, err := pipesock.BuildPipeListener(le, pipeRoot, runtimeUuid)
	if err != nil {
		return nil, err
	}

	// electron acts as the server (outbound=false)
	// we act as the client (outbound=true)
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)

	_ = os.Chmod(electronPath, 0o755) // try to chmod

	var electronArgs []string
	electronArgs = append(electronArgs, extraElectronFlags...)
	electronArgs = append(electronArgs, rendererPath)

	cmd := exec.NewCmd(electronPath, electronArgs...)
	cmd.Env = append(cmd.Env, "BLDR_RUNTIME_ID="+runtimeUuid)
	cmd.Stdout = le.WriterLevel(logrus.DebugLevel)
	cmd.Stderr = le.WriterLevel(logrus.DebugLevel)

	le.Debugf("starting electron: %s", cmd.String())
	err = cmd.Start()
	if err != nil {
		_ = pipeListener.Close()
		_ = smc.CloseWithErr(err)
		return nil, err
	}

	return &Electron{
		ctx: ctx,
		cmd: cmd,

		runtimeUuid:  runtimeUuid,
		electronPath: electronPath,
		rendererPath: rendererPath,

		ipc:      smc,
		listener: pipeListener,
	}, nil
}

// GetMuxedConn returns the muxed conn with the main process.
func (e *Electron) GetMuxedConn() network.MuxedConn {
	return e.ipc
}

// GetCmd returns the running Electron command.
func (e *Electron) GetCmd() *oexec.Cmd {
	return e.cmd
}

// Close shuts down the electron instance.
func (e *Electron) Close() {
	if e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_ = e.cmd.Wait()
	}
	if e.ipc != nil {
		_ = e.ipc.Close()
	}
	if e.listener != nil {
		_ = e.listener.Close()
	}
}

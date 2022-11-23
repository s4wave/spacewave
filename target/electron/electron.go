package electron

import (
	"context"
	oexec "os/exec"
	"path"

	"github.com/aperturerobotics/bldr/util/pipesock"
	singleton_muxed_conn "github.com/aperturerobotics/bldr/util/singleton-muxed-conn"
	"github.com/aperturerobotics/bldr/web/ipc"
	"github.com/aperturerobotics/util/exec"
	"github.com/sirupsen/logrus"
)

// Electron is a running instance of Electron.
type Electron struct {
	ctx context.Context
	cmd *oexec.Cmd

	runtimeUuid  string
	electronPath string
	rendererPath string

	ipc *singleton_muxed_conn.SingletonMuxedConn
}

// RunElectron listens on the IPC pipe and starts Electron sub-process.
func RunElectron(
	ctx context.Context,
	le *logrus.Entry,
	electronPath,
	workdirPath,
	rendererPath,
	runtimeUuid string,
) (*Electron, error) {
	le.Debug("listening on ipc socket")
	pipeRoot := rendererPath
	if !path.IsAbs(pipeRoot) {
		pipeRoot = path.Join(workdirPath, rendererPath)
	}
	pipeListener, err := pipesock.BuildPipeListener(le, pipeRoot, runtimeUuid)
	if err != nil {
		return nil, err
	}

	// electron acts as the server (outbound=false)
	// we act as the client (outbound=true)
	smc := singleton_muxed_conn.NewSingletonMuxedConn(ctx, true)
	go smc.AcceptPump(pipeListener)

	cmd := exec.NewCmd(electronPath, "--inspect=5858", rendererPath)
	cmd.Env = append(cmd.Env, "BLDR_RUNTIME_ID="+runtimeUuid)
	cmd.Dir = workdirPath
	cmd.Stderr = le.WriterLevel(logrus.DebugLevel)

	le.Debugf("starting electron: %s", cmd.String())
	err = cmd.Start()
	if err != nil {
		_ = smc.CloseWithErr(err)
		return nil, err
	}

	return &Electron{
		ctx: ctx,
		cmd: cmd,

		runtimeUuid:  runtimeUuid,
		electronPath: electronPath,
		rendererPath: rendererPath,

		ipc: smc,
	}, nil
}

// GetIpc returns the ipc.
func (e *Electron) GetIpc() ipc.IPC {
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
}

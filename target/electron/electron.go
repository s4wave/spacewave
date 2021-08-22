package electron

import (
	"context"
	oexec "os/exec"

	"github.com/aperturerobotics/controllerbus/util/exec"
	"github.com/sirupsen/logrus"
)

// Electron is a running instance of Electron.
type Electron struct {
	ctx          context.Context
	cmd          *oexec.Cmd
	ipcStream    *ipcStream
	sessionUUID  string
	electronPath string
	rendererPath string
}

// RunElectron listens on the IPC pipe and starts Electron sub-process.
func RunElectron(ctx context.Context, le *logrus.Entry, electronPath, rendererPath string) (*Electron, error) {
	sessionUuid := "xsessionidx" // TODO
	ipc, err := newIpcStream(ctx, le, rendererPath, sessionUuid)
	if err != nil {
		return nil, err
	}
	cmd := exec.NewCmd(electronPath, "--inspect=5858", "./")
	cmd.Env = append(cmd.Env, "BLDR_SESSION_ID="+sessionUuid)
	cmd.Dir = rendererPath
	le.Debugf("starting electron: %s", cmd.String())
	err = cmd.Start()
	if err != nil {
		ipc.Close()
		return nil, err
	}
	return &Electron{
		ctx:          ctx,
		cmd:          cmd,
		ipcStream:    ipc,
		sessionUUID:  sessionUuid,
		electronPath: electronPath,
		rendererPath: rendererPath,
	}, nil
}

// Close shuts down the electron instance.
func (e *Electron) Close() {
	if e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_ = e.cmd.Wait()
	}
	if e.ipcStream != nil {
		_ = e.ipcStream.Close()
	}
}

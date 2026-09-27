//go:build !js

package daemon

import (
	"context"
	"io"
	"net"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
)

// StartupNotifier transfers a child from launcher custody to daemon lifetime.
// Its methods run serially on the serving goroutine.
type StartupNotifier struct {
	// conn is the private startup pipe, nil after reporting or for manual serve.
	conn net.Conn
}

// NewStartupNotifier connects to the launcher's private pipe. An empty pipe ID
// selects manual serve, which needs no launcher acknowledgement.
func NewStartupNotifier(ctx context.Context, statePath, pipeID string) (*StartupNotifier, error) {
	// Manual serve owns its lifetime from invocation.
	if pipeID == "" {
		return &StartupNotifier{}, nil
	}
	conn, err := pipesock.DialPipeListener(ctx, NewStartupPipeLogger(), statePath, pipeID)
	if err != nil {
		return nil, err
	}
	return &StartupNotifier{conn: conn}, nil
}

// Ready reports a successfully initialized Resource service and waits for the
// parent's custody acknowledgement. Call before accepting public connections.
func (n *StartupNotifier) Ready(ctx context.Context) error {
	// Close the private pipe on cancellation; a missing acknowledgement aborts.
	if n.conn == nil {
		return nil
	}
	conn := n.conn
	defer n.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := io.WriteString(conn, "ready\n"); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return err
	}
	if ack[0] != 1 {
		return errors.New("invalid daemon readiness acknowledgement")
	}
	return nil
}

// Error reports a startup failure, preserving lease contention as a distinct
// event so a losing launcher can attach to the winner without takeover.
func (n *StartupNotifier) Error(err error) {
	// Reporting cannot replace the original startup failure.
	if n.conn == nil || err == nil {
		return
	}
	defer n.Close()
	message := "error: " + err.Error()
	if errors.Is(err, ErrStarting) {
		message = "starting"
	}
	_, _ = io.WriteString(n.conn, message+"\n")
}

// Close releases the private pipe without changing the daemon's lifetime.
func (n *StartupNotifier) Close() {
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
	}
}

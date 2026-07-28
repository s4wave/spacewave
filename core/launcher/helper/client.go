package launcher_helper

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	stderrors "errors"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// maxMessageSize is the maximum framed message size.
const maxMessageSize uint32 = 10 * 1024 * 1024

// Client manages a spacewave-helper subprocess with framed proto IPC.
type Client struct {
	le            *logrus.Entry
	cmd           *exec.Cmd
	conn          net.Conn
	listener      net.Listener
	pipeID        string
	helperCleanup func() error
	rootDir       string

	readMtx  sync.Mutex
	writeMtx sync.Mutex
}

// NewLoadingClient spawns the helper in --loading mode.
// rootDir is the application support directory.
// helperPath is the path to the spacewave-helper binary.
// iconPath is the path to the app icon file.
func NewLoadingClient(
	ctx context.Context,
	le *logrus.Entry,
	rootDir, helperPath, iconPath string,
) (*Client, error) {
	c := &Client{
		le:      le,
		rootDir: rootDir,
	}
	args := []string{"--loading"}
	if iconPath != "" {
		args = append(args, "--icon", iconPath)
	}
	if err := c.startHelper(ctx, helperPath, args...); err != nil {
		return nil, err
	}
	return c, nil
}

// NewUpdateClient spawns the helper in --update mode.
// currentPath is the path to the current .app or binary.
// stagedPath is the path to the staged update.
// pid is the current process PID.
func NewUpdateClient(
	ctx context.Context,
	le *logrus.Entry,
	rootDir, helperPath, currentPath, stagedPath string,
	pid int,
) (*Client, error) {
	c := &Client{
		le:      le,
		rootDir: rootDir,
	}
	pidStr := strconv.Itoa(pid)
	if err := c.startHelper(ctx, helperPath, "--update",
		"--current", currentPath,
		"--staged", stagedPath,
		"--pid", pidStr); err != nil {
		return nil, err
	}
	return c, nil
}

// startHelper creates a pipesock listener, spawns the helper, and waits
// for the HelperReady event.
func (c *Client) startHelper(ctx context.Context, helperPath string, args ...string) error {
	// Generate a random pipe ID.
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return errors.Wrap(err, "generate pipe id")
	}
	c.pipeID = hex.EncodeToString(b)

	// Create listener via pipesock (handles Unix sockets and Windows named pipes).
	var err error
	c.listener, err = pipesock.BuildPipeListener(c.le, c.rootDir, c.pipeID)
	if err != nil {
		return errors.Wrap(err, "listen pipe")
	}

	// Pass the root dir and pipe ID so the helper can connect via pipesock conventions.
	args = append(args, "--pipe-root", c.rootDir, "--pipe-id", c.pipeID)

	helperPath, cleanup, err := prepareHelperExecutable(c.rootDir, helperPath)
	if err != nil {
		closeErr := c.listener.Close()
		c.listener = nil
		return stderrors.Join(
			errors.Wrap(err, "prepare helper"),
			errors.Wrap(closeErr, "close helper listener"),
		)
	}
	c.helperCleanup = cleanup

	// Spawn the helper process.
	c.cmd = exec.CommandContext(ctx, helperPath, args...)
	prepareHelperCommand(c.cmd)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	if err := c.cmd.Start(); err != nil {
		return stderrors.Join(
			errors.Wrap(err, "start helper"),
			errors.Wrap(c.Close(), "clean up helper"),
		)
	}

	// Accept the helper connection.
	c.conn, err = c.listener.Accept()
	if err != nil {
		return stderrors.Join(
			errors.Wrap(err, "accept helper connection"),
			errors.Wrap(c.Close(), "clean up helper"),
		)
	}

	// Wait for HelperReady.
	evt, err := c.RecvEvent(ctx)
	if err != nil {
		return stderrors.Join(
			errors.Wrap(err, "wait for helper ready"),
			errors.Wrap(c.Close(), "clean up helper"),
		)
	}
	if evt.GetReady() == nil {
		return stderrors.Join(
			errors.New("expected HelperReady, got different event"),
			errors.Wrap(c.Close(), "clean up helper"),
		)
	}

	c.le.Debug("helper connected and ready")
	return nil
}

// SendProgress sends a progress update to the helper.
// fraction < 0 means indeterminate.
func (c *Client) SendProgress(fraction float32, text string) error {
	return c.sendMessage(&HelperMessage{
		Body: &HelperMessage_Progress{
			Progress: &ProgressUpdate{
				Fraction: fraction,
				Text:     text,
			},
		},
	})
}

// SendStatus sends a status text update.
func (c *Client) SendStatus(text string) error {
	return c.sendMessage(&HelperMessage{
		Body: &HelperMessage_Status{
			Status: &StatusUpdate{
				Text: text,
			},
		},
	})
}

// SendDismiss tells the helper to close its window and exit.
func (c *Client) SendDismiss() error {
	return c.sendMessage(&HelperMessage{
		Body: &HelperMessage_Dismiss{
			Dismiss: &DismissCommand{},
		},
	})
}

// SendError sends an error with optional retry.
func (c *Client) SendError(msg string, retryable bool) error {
	return c.sendMessage(&HelperMessage{
		Body: &HelperMessage_Error{
			Error: &ErrorReport{
				Message:   msg,
				Retryable: retryable,
			},
		},
	})
}

// RecvEvent blocks until the helper sends an event.
func (c *Client) RecvEvent(ctx context.Context) (*HelperEvent, error) {
	data, err := c.readFrame()
	if err != nil {
		return nil, err
	}
	evt := &HelperEvent{}
	if err := evt.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal helper event")
	}
	return evt, nil
}

// Close terminates the helper subprocess and cleans up.
func (c *Client) Close() error {
	var closeErr error
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
			closeErr = stderrors.Join(closeErr, errors.Wrap(err, "close helper connection"))
		}
	}
	if c.listener != nil {
		if err := c.listener.Close(); err != nil && !stderrors.Is(err, net.ErrClosed) {
			closeErr = stderrors.Join(closeErr, errors.Wrap(err, "close helper listener"))
		}
		c.listener = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		if err := c.cmd.Process.Kill(); err != nil && !stderrors.Is(err, os.ErrProcessDone) {
			closeErr = stderrors.Join(closeErr, errors.Wrap(err, "kill helper process"))
		}
		if err := c.cmd.Wait(); err != nil {
			c.le.WithError(err).Debug("helper process exited")
		}
		c.cmd = nil
	}
	if c.helperCleanup != nil {
		closeErr = stderrors.Join(closeErr, c.helperCleanup())
		c.helperCleanup = nil
	}
	return closeErr
}

// sendMessage marshals and writes a framed HelperMessage.
func (c *Client) sendMessage(msg *HelperMessage) error {
	data, err := msg.MarshalVT()
	if err != nil {
		return err
	}
	return c.writeFrame(data)
}

// writeFrame writes data with a 4-byte LE uint32 length prefix.
func (c *Client) writeFrame(data []byte) error {
	c.writeMtx.Lock()
	defer c.writeMtx.Unlock()

	if len(data) > math.MaxUint32 {
		return errors.New("message too large")
	}

	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := c.conn.Write(lenBuf); err != nil {
		return err
	}
	_, err := c.conn.Write(data)
	return err
}

// readFrame reads a 4-byte LE length-prefixed frame.
func (c *Client) readFrame() ([]byte, error) {
	c.readMtx.Lock()
	defer c.readMtx.Unlock()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf)
	if msgLen > maxMessageSize {
		return nil, errors.New("message exceeds max size")
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}
	return data, nil
}

//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// objectRPCArgs selects the typed object whose service object rpc serves.
type objectRPCArgs struct {
	// statePath is the daemon state directory.
	statePath string
	// spaceID overrides the Space in the URI.
	spaceID string
	// sessionIdx overrides the session in the URI when positive.
	sessionIdx int
}

// newObjectRPCCommand builds the object rpc subcommand.
func newObjectRPCCommand() *cli.Command {
	args := &objectRPCArgs{}
	return &cli.Command{
		Name:      "rpc",
		Usage:     "serve a typed object's RPC service on stdin and stdout",
		ArgsUsage: "<uri>",
		Description: fsURIDescription(
			"Mount the typed object and serve its starpc services to the process at the\n"+
				"other end of stdin and stdout, as a yamux session carrying one RPC per\n"+
				"stream. The command exits when stdin closes. It exits with an error when\n"+
				"the daemon connection or the object ends. It writes only the session to\n"+
				"stdout.",
			`  # open the SQL database object my-db for a client process
  spacewave space object rpc my-db`,
		),
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns flags for object rpc.
func (a *objectRPCArgs) BuildFlags() []cli.Flag {
	return commonFsFlags(&a.statePath, &a.spaceID, &a.sessionIdx)
}

// Run serves the object's RPC service until stdin closes, or fails when the
// daemon connection or the object's resource ends.
func (a *objectRPCArgs) Run(c *cli.Context) error {
	// Parse the object URI.
	uri, err := parseFsURI(c.Args().First(), a.spaceID, a.sessionIdx)
	if err != nil {
		return err
	}
	if uri.path != "" {
		return errors.New("object rpc takes an object, not a path within it")
	}

	// Mount the typed object.
	mount, err := mountObjectChain(c, a.statePath, uri, nil)
	if err != nil {
		return err
	}
	defer mount.release()

	// Forward each stream the peer opens to the object's service.
	ctx, cancel := context.WithCancel(c.Context)
	defer cancel()
	conn, err := srpc.NewMuxedConnWithRwc(ctx, stdio{}, false, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Close the session when the object's resource ends. The resource client
	// releases every reference when the daemon connection ends.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-mount.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	err = srpc.NewServer(srpc.NewClientInvoker(mount.TypedClient)).AcceptMuxedConn(ctx, conn)

	// Report why the object ended, or end cleanly when the peer closed stdin.
	if resourceErr := mount.resourceEndError(); resourceErr != nil {
		return resourceErr
	}
	if err == io.EOF || conn.IsClosed() {
		return nil
	}
	return err
}

// resourceEndError reports whether the daemon connection or the object ended.
func (c *sdkClient) resourceEndError(objectDone <-chan struct{}, objectKey string) error {
	select {
	case <-c.resClient.Done():
		return errors.New("daemon connection closed")
	case <-objectDone:
		return errors.New("object " + objectKey + " was released")
	default:
		return nil
	}
}

// stdio is the process's stdin and stdout as one stream.
type stdio struct{}

// Read reads from stdin.
func (stdio) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

// Write writes to stdout.
func (stdio) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

// Close closes stdout, which tells the peer the session ended.
func (stdio) Close() error {
	return os.Stdout.Close()
}

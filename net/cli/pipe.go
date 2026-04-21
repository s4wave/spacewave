package cli

import (
	"context"
	"crypto/rand"
	"io"
	"log"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/daemon"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	udptpt "github.com/s4wave/spacewave/net/transport/udp"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

// DefaultPipeProtocolID is the default protocol ID for pipe streams.
const DefaultPipeProtocolID = "/pipe/stream"

// PipeArgs contains the arguments for the pipe command.
type PipeArgs struct {
	// ListenAddr is the address to listen on (server mode).
	// Example: ":5112"
	ListenAddr string
	// ConnectAddr is the address to connect to (client mode).
	// Format: "peer-id@host:port"
	ConnectAddr string
	// ProtocolID is the stream protocol ID.
	ProtocolID string
	// PrivKeyPath is the path to the private key file.
	// If empty, a key will be generated.
	PrivKeyPath string
	// Quiet suppresses status messages to stderr.
	Quiet bool
}

// BuildFlags returns the CLI flags for the pipe command.
func (a *PipeArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "listen",
			Aliases:     []string{"l"},
			Usage:       "listen address for server mode (e.g., :5112)",
			Destination: &a.ListenAddr,
		},
		&cli.StringFlag{
			Name:        "connect",
			Aliases:     []string{"c"},
			Usage:       "connect address for client mode (peer-id@host:port)",
			Destination: &a.ConnectAddr,
		},
		&cli.StringFlag{
			Name:        "protocol-id",
			Aliases:     []string{"p"},
			Usage:       "stream protocol ID",
			Value:       DefaultPipeProtocolID,
			Destination: &a.ProtocolID,
		},
		&cli.StringFlag{
			Name:        "key",
			Aliases:     []string{"k"},
			Usage:       "path to private key file (auto-generated if empty)",
			Destination: &a.PrivKeyPath,
		},
		&cli.BoolFlag{
			Name:        "quiet",
			Aliases:     []string{"q"},
			Usage:       "suppress status messages",
			Destination: &a.Quiet,
		},
	}
}

// Run executes the pipe command.
func (a *PipeArgs) Run(c *cli.Context) error {
	ctx := c.Context

	// Validate arguments
	if a.ListenAddr == "" && a.ConnectAddr == "" {
		return errors.New("must specify either -l (listen) or -c (connect)")
	}
	if a.ListenAddr != "" && a.ConnectAddr != "" {
		return errors.New("cannot specify both -l (listen) and -c (connect)")
	}

	if a.ListenAddr != "" {
		return a.runListen(ctx)
	}
	return a.runConnect(ctx)
}

// setupDaemon creates an in-process daemon with UDP transport.
// Returns the daemon and a cleanup function that releases controller references.
func (a *PipeArgs) setupDaemon(ctx context.Context, listenAddr string, dialers map[string]*dialer.DialerOpts) (*daemon.Daemon, func(), error) {
	// Create logger (quiet mode uses a no-op logger)
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	if a.Quiet {
		logger.SetOutput(io.Discard)
	}
	le := logrus.NewEntry(logger)

	// Generate or load private key
	privKey, err := a.loadOrGenerateKey(le)
	if err != nil {
		return nil, nil, errors.Wrap(err, "load/generate key")
	}

	// Create daemon
	d, err := daemon.NewDaemon(ctx, privKey, daemon.ConstructOpts{
		LogEntry: le,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "construct daemon")
	}

	b := d.GetControllerBus()
	sr := d.GetStaticResolver()

	// Add hold-open controller (prevents link timeout during streaming)
	sr.AddFactory(link_holdopen_controller.NewFactory(b))
	_, _, hoRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&link_holdopen_controller.Config{}),
		nil,
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "start hold-open controller")
	}

	// Start UDP transport
	_, _, udpRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&udptpt.Config{
			ListenAddr: listenAddr,
			Dialers:    dialers,
		}),
		nil,
	)
	if err != nil {
		hoRef.Release()
		return nil, nil, errors.Wrap(err, "start UDP transport")
	}

	// Return cleanup function that releases all controller references
	cleanup := func() {
		udpRef.Release()
		hoRef.Release()
	}

	return d, cleanup, nil
}

// loadOrGenerateKey loads a private key from file or generates a new one.
func (a *PipeArgs) loadOrGenerateKey(le *logrus.Entry) (crypto.PrivKey, error) {
	if a.PrivKeyPath != "" {
		// Use keyfile helper to load or generate key
		return keyfile.OpenOrWritePrivKey(le, a.PrivKeyPath)
	}

	// No path specified, just generate an ephemeral key
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate key")
	}
	return privKey, nil
}

// parseConnectAddr parses a "peer-id@host:port" string.
func parseConnectAddr(addr string) (peer.ID, string, error) {
	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 {
		return "", "", errors.New("connect address must be in format peer-id@host:port")
	}

	peerIDStr := strings.TrimSpace(parts[0])
	hostPort := strings.TrimSpace(parts[1])

	if peerIDStr == "" {
		return "", "", errors.New("peer ID cannot be empty")
	}
	if hostPort == "" {
		return "", "", errors.New("host:port cannot be empty")
	}

	peerID, err := confparse.ParsePeerID(peerIDStr)
	if err != nil {
		return "", "", errors.Wrap(err, "parse peer ID")
	}

	return peerID, hostPort, nil
}

// pipeStream performs bidirectional copying between a stream and stdin/stdout.
func pipeStream(strm io.ReadWriteCloser, stdin io.Reader, stdout io.Writer) error {
	done := make(chan struct{})

	// stdin -> stream
	go func() {
		buf := make([]byte, 8192)
		_, _ = io.CopyBuffer(strm, stdin, buf)
		// Signal that stdin is done - close the stream to signal EOF to remote
		strm.Close()
	}()

	// stream -> stdout (blocks until stream closes or errors)
	go func() {
		buf := make([]byte, 8192)
		_, _ = io.CopyBuffer(stdout, strm, buf)
		close(done)
	}()

	// Wait for stream->stdout to complete
	<-done
	return nil
}

// logStatus prints a status message to stderr if not in quiet mode.
func (a *PipeArgs) logStatus(format string, args ...any) {
	if !a.Quiet {
		log.Printf(format, args...)
	}
}

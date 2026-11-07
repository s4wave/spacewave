// Command spacewave-storage-node holds a store's new blocks on a second disk.
//
// It takes each publication's blocks over one connection, answers once they are
// durable in a local pack log, uploads the finished packs to an SFTP pack store
// and reports each one back. It listens on a loopback address: the connection
// arrives through an SSH tunnel, which carries the authentication.
package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// uploadRetry is how long a failed drain waits before it runs again.
const uploadRetry = 5 * time.Second

// args are the command's flags.
type args struct {
	listen   string
	dir      string
	sftpAddr string
	hostKey  string
	netrc    string
	root     string
	prefix   string
	conns    int
	keepMiB  int64
	minFree  uint64
	backfill bool
	linger   time.Duration
}

// main runs the node until it is signalled.
func main() {
	// Describe the flags.
	a := &args{}
	app := cli.NewApp()
	app.Name = "spacewave-storage-node"
	app.Usage = "hold a store's new blocks on a second disk and upload them"
	app.Flags = []cli.Flag{
		&cli.StringFlag{Name: "listen", Usage: "loopback address to listen on", Value: "127.0.0.1:7140", Destination: &a.listen},
		&cli.StringFlag{Name: "dir", Usage: "directory of the pack log and its reports", Required: true, Destination: &a.dir},
		&cli.StringFlag{Name: "sftp-addr", Usage: "SFTP server host:port", Required: true, Destination: &a.sftpAddr},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "SFTP server host key", Required: true, Destination: &a.hostKey},
		&cli.StringFlag{Name: "netrc", Usage: "netrc file holding the SFTP login", Required: true, Destination: &a.netrc},
		&cli.StringFlag{Name: "root", Usage: "SFTP root of the pack store", Required: true, Destination: &a.root},
		&cli.StringFlag{Name: "prefix", Usage: "object prefix of the pack store", Destination: &a.prefix},
		&cli.IntFlag{Name: "conns", Usage: "SFTP connections", Value: 2, Destination: &a.conns},
		&cli.Int64Flag{Name: "keep-mib", Usage: "uploaded packs the log keeps; bounds the memory its table takes", Value: 8192, Destination: &a.keepMiB},
		&cli.Uint64Flag{Name: "min-free-gib", Usage: "keep and fill no read copies with less free disk than this", Value: 200, Destination: &a.minFree},
		&cli.BoolFlag{Name: "backfill", Usage: "fill every pack the remote lists, one at a time", Destination: &a.backfill},
		&cli.DurationFlag{Name: "linger", Usage: "rotate a pack once its first batch is this old", Value: 30 * time.Second, Destination: &a.linger},
	}
	app.Action = func(*cli.Context) error {
		// Stop on a signal.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return a.run(ctx)
	}

	// Run it.
	if err := app.Run(os.Args); err != nil {
		logrus.WithError(err).Fatal("storage node stopped")
	}
}

// run serves until ctx ends.
func (a *args) run(ctx context.Context) error {
	// Refuse a listener a tunnel would not protect.
	le := logrus.NewEntry(logrus.StandardLogger())
	host, _, err := net.SplitHostPort(a.listen)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.Errorf("listen address %q is not loopback", a.listen)
	}

	// Open the pack log and the server in front of it.
	log, err := packahead.Open(ctx, filepath.Join(a.dir, "log"), packahead.Options{Linger: a.linger})
	if err != nil {
		return errors.Wrap(err, "open pack log")
	}
	defer log.Close()
	server, err := block_store_nodepush.NewServer(log, filepath.Join(a.dir, "reports"))
	if err != nil {
		return err
	}

	// Reach the pack store. It keeps no index: the node only adds packs.
	client, remote, err := a.openRemote(le)
	if err != nil {
		return err
	}
	defer remote.Close()

	// Serve reads from read copies of the packs, while the disk has room.
	packDir := a.prefix + "packs"
	packs, err := block_store_nodepush.NewPackFiles(le, filepath.Join(a.dir, "packs"), client, a.root, packDir)
	if err != nil {
		return err
	}
	packs.SetRoom(func() bool { return a.hasRoom(le) })
	server.SetPacks(packs)
	go packs.Run(ctx)
	if a.backfill {
		go a.fillAll(ctx, le, packs, client, packDir)
	}

	// Upload finished packs until ctx ends.
	go a.drain(ctx, le, log, block_store_nodepush.NewReportingRemote(remote, server))

	// Serve one connection at a time; a newer one replaces the older.
	return serve(ctx, le, a.listen, server)
}

// openRemote opens the SFTP pack store.
func (a *args) openRemote(le *logrus.Entry) (*block_store_sftp.Client, *block_store_s3.PackStore, error) {
	// Read the login, as the store's own host does.
	host, _, err := net.SplitHostPort(a.sftpAddr)
	if err != nil {
		return nil, nil, err
	}
	logins, err := block_store_ftps.ReadNetrc(a.netrc, host)
	if err != nil {
		return nil, nil, err
	}

	// Connect, and keep every pack.
	client, err := block_store_sftp.NewClient(block_store_sftp.Config{
		Addr:     a.sftpAddr,
		User:     logins[0].User,
		Password: logins[0].Password,
		HostKey:  a.hostKey,
		Conns:    a.conns,
	})
	if err != nil {
		return nil, nil, err
	}
	store := block_store_s3.NewPackStore(le, client, a.root, a.prefix, &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true})
	return client, store, nil
}

// hasRoom reports whether the disk keeps its free space past a read copy.
func (a *args) hasRoom(le *logrus.Entry) bool {
	// Assume room when the disk can't say: a full disk fails only the copy.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(a.dir, &stat); err != nil {
		le.WithError(err).Warn("read the free disk space")
		return true
	}
	return stat.Bavail*uint64(stat.Bsize) >= a.minFree<<30 //nolint:gosec // a block size is positive
}

// fillAll fills every pack the remote lists.
func (a *args) fillAll(ctx context.Context, le *logrus.Entry, packs *block_store_nodepush.PackFiles, client *block_store_sftp.Client, packDir string) {
	// Walk the listing, handing each name on as it comes.
	err := packs.Backfill(ctx, func(fn func(id string) error) error {
		return client.ListObjects(ctx, a.root, packDir+"/", func(key string, _ int64) error {
			return fn(path.Base(key))
		})
	})
	le.WithError(err).Info("backfill ended")
}

// drain uploads the log's finished packs until ctx ends, retrying a failure.
func (a *args) drain(ctx context.Context, le *logrus.Entry, log *packahead.Log, remote packahead.Remote) {
	drain := packahead.NewDrain(log, packahead.DrainOptions{KeepBytes: a.keepMiB << 20})
	for ctx.Err() == nil {
		// Run, and pause after a failure.
		err := drain.Run(ctx, remote)
		if ctx.Err() != nil {
			return
		}
		le.WithError(err).Warn("upload packs")
		select {
		case <-ctx.Done():
		case <-time.After(uploadRetry):
		}
	}
}

// serve accepts connections on addr until ctx ends.
func serve(ctx context.Context, le *logrus.Entry, addr string, server *block_store_nodepush.Server) error {
	// Listen, closing the listener with ctx.
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	le.WithField("addr", addr).Info("storage node listening")

	// Serve each connection beside the next accept.
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			err := server.Serve(ctx, conn)
			le.WithError(err).Debug("connection ended")
		}()
	}
}

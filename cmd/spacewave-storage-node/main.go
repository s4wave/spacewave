// Command spacewave-storage-node holds a store's new blocks on a second disk.
//
// It takes each publication's blocks over one connection, answers once they are
// durable in a local pack log, uploads the finished packs to an SFTP pack store
// and reports each one back. It serves the store's block reads from read
// copies of the SFTP packs, range-reading SFTP for a pack it lacks while it
// fetches the whole pack, and never uploads a read copy. It listens on a
// loopback address: the connection arrives through an SSH tunnel, which
// carries the authentication. Its metrics are served on a second loopback
// address.
package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	"github.com/sirupsen/logrus"
)

// args are the command's flags.
type args struct {
	listen        string
	metricsListen string
	dir           string
	sftpAddr      string
	hostKey       string
	netrc         string
	root          string
	prefix        string
	conns         int
	remoteReads   int
	keepMiB       int64
	minFree       uint64
	pruneFree     uint64
	tunables      string
	linger        time.Duration
}

// tunables are the node's knobs an operator changes while it runs, by editing
// the JSON file --tunables names and sending the process SIGHUP. A missing file
// keeps every default.
type tunables struct {
	// backfill is "on" to fill every pack the remote lists into read copies;
	// anything else, the default, leaves it off. Its JSON key is backfill.
	backfill string
	// backfillMiBs paces the backfill in MiB a second; zero keeps 8. Its JSON
	// key is backfill_mib_s.
	backfillMiBs float64
	// readCopies is "off" to keep no uploaded pack and fetch no whole pack on
	// a read miss; anything else keeps both on. Its JSON key is read_copies.
	readCopies string
	// uploadMiBs paces the upload in MiB a second; zero uploads each pack at
	// once. Its JSON key is upload_mib_s.
	uploadMiBs float64
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
		&cli.StringFlag{Name: "metrics-listen", Usage: "loopback address serving /metrics; empty serves none", Value: "127.0.0.1:7141", Destination: &a.metricsListen},
		&cli.StringFlag{Name: "dir", Usage: "directory of the pack log, its reports and the read copies", Required: true, Destination: &a.dir},
		&cli.StringFlag{Name: "sftp-addr", Usage: "SFTP server host:port", Required: true, Destination: &a.sftpAddr},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "SFTP server host key", Required: true, Destination: &a.hostKey},
		&cli.StringFlag{Name: "netrc", Usage: "netrc file holding the SFTP login", Required: true, Destination: &a.netrc},
		&cli.StringFlag{Name: "root", Usage: "SFTP root of the pack store", Required: true, Destination: &a.root},
		&cli.StringFlag{Name: "prefix", Usage: "object prefix of the pack store", Destination: &a.prefix},
		&cli.IntFlag{Name: "conns", Usage: "SFTP connections", Value: 2, Destination: &a.conns},
		&cli.IntFlag{Name: "remote-reads", Usage: "range reads of SFTP at once for packs the node lacks; zero uses one per connection", Destination: &a.remoteReads},
		&cli.Int64Flag{Name: "keep-mib", Usage: "uploaded packs the log keeps; bounds the memory its table takes", Value: 8192, Destination: &a.keepMiB},
		&cli.Uint64Flag{Name: "min-free-gib", Usage: "keep and fill no read copies with less free disk than this", Value: 200, Destination: &a.minFree},
		&cli.Uint64Flag{Name: "prune-free-gib", Usage: "below this free disk, prune read copies oldest first back to min-free-gib; zero never prunes", Value: 100, Destination: &a.pruneFree},
		&cli.StringFlag{Name: "tunables", Usage: "JSON file of live knobs, reread on SIGHUP: backfill, backfill_mib_s, read_copies, upload_mib_s", Destination: &a.tunables},
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
	if err := loopback(a.listen); err != nil {
		return err
	}
	if a.metricsListen != "" {
		if err := loopback(a.metricsListen); err != nil {
			return err
		}
	}
	if a.pruneFree >= a.minFree && a.pruneFree != 0 {
		return errors.New("prune-free-gib must sit below min-free-gib, so fills stop before pruning starts")
	}

	// Reach the pack store. It keeps no index: the node only adds packs.
	client, remote, err := a.openRemote(le)
	if err != nil {
		return err
	}
	defer remote.Close()

	// Open the node: its pack log, reports and read copies.
	remoteReads := a.remoteReads
	if remoteReads <= 0 {
		remoteReads = a.conns
	}
	packDir := a.prefix + "packs"
	node, err := block_store_nodepush.OpenNode(ctx, le, block_store_nodepush.NodeConfig{
		Dir:       a.dir,
		Linger:    a.linger,
		KeepBytes: a.keepMiB << 20,
		Remote:    remote,
		Source:    client,
		Bucket:    a.root,
		Prefix:    packDir,
		List: func(ctx context.Context, fn func(id string, size int64) error) error {
			return client.ListObjects(ctx, a.root, packDir+"/", func(key string, size int64) error {
				return fn(path.Base(key), size)
			})
		},
		RemoteReads:    remoteReads,
		MinFreeBytes:   a.minFree << 30,
		PruneFreeBytes: a.pruneFree << 30,
		FreeBytes:      func() (uint64, bool) { return freeBytes(a.dir) },
	})
	if err != nil {
		return err
	}
	defer node.Close()

	// Apply the tunables now and on each SIGHUP.
	a.applyTunables(le, node)
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hup:
				a.applyTunables(le, node)
			}
		}
	}()

	// Run the node's background work, and serve its metrics.
	running := make(chan struct{})
	go func() {
		defer close(running)
		node.Run(ctx)
	}()
	defer func() { <-running }()
	if a.metricsListen != "" {
		go serveMetrics(ctx, le, a.metricsListen, node)
	}

	// Serve one connection at a time; a newer one replaces the older.
	return serve(ctx, le, a.listen, node)
}

// loopback refuses an address that is not on the loopback interface.
func loopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("listen address " + addr + " is not loopback")
	}
	return nil
}

// applyTunables reads the tunables file and applies it. A missing file leaves
// every knob at its default, and a bad one keeps the knobs as they are.
func (a *args) applyTunables(le *logrus.Entry, node *block_store_nodepush.Node) {
	// Read the file, if any.
	var t tunables
	if a.tunables != "" {
		data, err := os.ReadFile(a.tunables)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			le.WithError(err).Warn("read the tunables file, keeping the knobs")
			return
		default:
			var p fastjson.Parser
			v, err := p.ParseBytes(data)
			if err != nil {
				le.WithError(err).Warn("parse the tunables file, keeping the knobs")
				return
			}
			t.backfill = string(v.GetStringBytes("backfill"))
			t.backfillMiBs = v.GetFloat64("backfill_mib_s")
			t.readCopies = string(v.GetStringBytes("read_copies"))
			t.uploadMiBs = v.GetFloat64("upload_mib_s")
		}
	}

	// Apply them.
	on := t.backfill == "on"
	rate := int64(t.backfillMiBs * (1 << 20))
	node.SetBackfill(on, rate)
	node.SetCopies(t.readCopies != "off")
	node.SetUploadRate(int64(t.uploadMiBs * (1 << 20)))
	le.WithFields(logrus.Fields{
		"backfill":       on,
		"backfill-mib-s": t.backfillMiBs,
		"read-copies":    t.readCopies != "off",
		"upload-mib-s":   t.uploadMiBs,
	}).Info("storage node tunables")
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

// freeBytes reports the free disk of dir.
func freeBytes(dir string) (uint64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, false
	}
	return stat.Bavail * uint64(stat.Bsize), true //nolint:gosec // a block size is positive
}

// serveMetrics serves the node's metrics on addr until ctx ends.
func serveMetrics(ctx context.Context, le *logrus.Entry, addr string, node *block_store_nodepush.Node) {
	// Answer /metrics alone.
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_ = node.WriteMetrics(w)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	// Close it with ctx.
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		le.WithError(err).Warn("serve metrics")
	}
}

// serve accepts connections on addr until ctx ends.
func serve(ctx context.Context, le *logrus.Entry, addr string, node *block_store_nodepush.Node) error {
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
			err := node.Serve(ctx, conn)
			le.WithError(err).Debug("connection ended")
		}()
	}
}

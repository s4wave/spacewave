//go:build !js

package spacewave_cli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
)

// ftpsArgs locate an FTPS server and its login.
type ftpsArgs struct {
	// host is the server's host or host:port.
	host string
	// netrc is the netrc file holding the login of host.
	netrc string
	// root is the directory holding the objects, relative to the login
	// directory.
	root string
	// conns is the number of control connections.
	conns int
}

// client reads the login from the netrc file and builds the FTPS client.
func (a *ftpsArgs) client() (*block_store_ftps.Client, error) {
	// Require the login file and the root directory.
	if a.netrc == "" || a.root == "" {
		return nil, errors.New("--ftps-host needs --ftps-netrc and --ftps-root")
	}

	// Read the login of the host without its port.
	host := a.host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	logins, err := block_store_ftps.ReadNetrc(a.netrc, host)
	if err != nil {
		return nil, err
	}
	return block_store_ftps.NewClient(block_store_ftps.Config{Addr: a.host, Logins: logins, MaxSessions: a.conns}), nil
}

// smokeObjectSize is the size of the objects the smoke test writes, about
// one pack.
const smokeObjectSize = 11 << 20

// newDebugFTPSSmokeCommand builds the hidden command that measures an FTPS
// server as a pack store.
func newDebugFTPSSmokeCommand() *cli.Command {
	var args ftpsArgs
	return &cli.Command{
		Name:   "ftps-smoke",
		Usage:  "write, read, list and delete scratch objects on an FTPS server and time each",
		Hidden: true,
		Description: "Writes one 11 MiB object, then one per connection in parallel, under --ftps-root, reads a 4 KiB " +
			"and a 1 MiB range, lists them, and deletes every object it wrote. Use a scratch directory.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "ftps-host", Usage: "the FTPS server", Destination: &args.host, Required: true},
			&cli.StringFlag{Name: "ftps-netrc", Usage: "the netrc file holding the FTPS login", Destination: &args.netrc, Required: true},
			&cli.StringFlag{Name: "ftps-root", Usage: "the scratch directory", Destination: &args.root, Required: true},
			&cli.IntFlag{Name: "ftps-conns", Usage: "the most FTPS sessions open at once across the netrc logins, spares included", Value: block_store_ftps.DefaultMaxSessions, Destination: &args.conns},
		},
		Action: func(c *cli.Context) error {
			client, err := args.client()
			if err != nil {
				return err
			}
			defer client.Close()
			return ftpsSmoke(c.Context, client, args.root, client.Conns(), os.Stderr)
		},
	}
}

// ftpsSmoke runs the smoke test on client under root with conns parallel
// writes, writing timings to w. It deletes what it wrote even on failure.
func ftpsSmoke(ctx context.Context, client *block_store_ftps.Client, root string, conns int, w io.Writer) error {
	// Fill one object with random bytes and delete every written key on return.
	data := make([]byte, smokeObjectSize)
	_, _ = rand.Read(data)
	keys := []string{"smoke/single"}
	defer func() {
		for _, key := range keys {
			if err := client.DeleteObject(context.WithoutCancel(ctx), root, key); err != nil {
				fmt.Fprintf(w, "delete %s: %v\n", key, err)
			}
		}
	}()

	// Write one object, creating its directory.
	start := time.Now()
	if err := client.PutObject(ctx, root, keys[0], data, ""); err != nil {
		return err
	}
	fmt.Fprintf(w, "put 1 x %s: %s, %s/s\n", humanize.IBytes(smokeObjectSize), time.Since(start).Round(time.Millisecond), rate(smokeObjectSize, time.Since(start)))

	// Write one object per connection in parallel.
	errs := make(chan error, conns)
	start = time.Now()
	for i := range conns {
		key := "smoke/parallel-" + strconv.Itoa(i)
		keys = append(keys, key)
		go func() { errs <- client.PutObject(ctx, root, key, data, "") }()
	}
	var err error
	for range conns {
		err = errors.Join(err, <-errs)
	}
	if err != nil {
		return err
	}
	total := int64(conns) * smokeObjectSize
	fmt.Fprintf(w, "put %d x %s in parallel: %s, %s/s\n", conns, humanize.IBytes(smokeObjectSize), time.Since(start).Round(time.Millisecond), rate(total, time.Since(start)))

	// Read more small ranges than the pool holds connections, since a server
	// may end the session of each early-closed download, then large ranges.
	for _, r := range []struct{ size, count int }{{4 << 10, 3 * conns}, {1 << 20, 4}} {
		var sum, slowest time.Duration
		for i := range r.count {
			off := int64(i * smokeObjectSize / r.count / 2)
			start = time.Now()
			got, err := client.GetObjectRange(ctx, root, keys[0], off, r.size)
			if err != nil {
				return err
			}
			if string(got) != string(data[off:off+int64(r.size)]) {
				return fmt.Errorf("range of %d bytes read %d bytes that differ", r.size, len(got))
			}
			took := time.Since(start)
			sum, slowest = sum+took, max(slowest, took)
		}
		fmt.Fprintf(w, "range %s x %d: mean %s, max %s\n", humanize.IBytes(uint64(r.size)), r.count, //nolint:gosec // sizes are positive constants.
			(sum / time.Duration(r.count)).Round(time.Millisecond), slowest.Round(time.Millisecond))
	}

	// List the objects.
	start = time.Now()
	var listed int
	err = client.ListObjects(ctx, root, "smoke/", func(key string, size int64) error {
		if size != smokeObjectSize {
			return fmt.Errorf("%s lists %d bytes", key, size)
		}
		listed++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "list %d objects: %s\n", listed, time.Since(start).Round(time.Millisecond))
	if listed != len(keys) {
		return fmt.Errorf("listed %d objects; wrote %d", listed, len(keys))
	}
	return nil
}

// rate formats n bytes over d as a size per second.
func rate(n int64, d time.Duration) string {
	return humanize.Bytes(uint64(float64(n) / d.Seconds()))
}

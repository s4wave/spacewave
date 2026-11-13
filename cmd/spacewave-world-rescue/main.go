//go:build !js && !wasip1 && !windows

// Command spacewave-world-rescue fetches a list of blocks from the packfiles a
// PackStore wrote to an FTPS server, verifies each against its ref, and
// writes the verified blocks to a local pack set in the PackStore layout
// (packs/{id} and entries/{id}), or with --layout adopt flat as {id} and
// {id}.entry, the layout hyperline-backend world-adopt-packs reads.
//
// It places the blocks through a read-only copy of the writer's pack index,
// reads each pack whole, keeping only the wanted blocks, most wanted packs
// first, within a session cap, rate caps and a byte cap kept across runs, and stops
// at once on a login refusal or a session limit. An append-only ledger lets a
// stopped run resume. It never deletes, local or remote.
package main

import (
	"cmp"
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aperturerobotics/cli"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// maxSessions is the most FTPS sessions a run may open: one below the
// server's per-login limit of ten.
const maxSessions = 9

// defaultIndexTable is the table of the pack index file.
const defaultIndexTable = "pack_index"

// Exit codes past the generic failure.
const (
	exitHardStop = 2
	exitStopped  = 3
)

// args are the command's flags.
type args struct {
	refs, index, indexTable, storeID string
	indexScratch                     string
	addr, netrc, root, prefix        string
	out, ledgerPath, layout          string
	sessions, minWanted, retries     int
	maxPacks, maxDials               int
	ranges, retryFailed, stopOnDrop  bool
	packList                         string
	overhead, maxBytes, maxPackMiB   uint64
	diskFloorGB                      float64
	timeout, backoff, maxBackoff     time.Duration
	progress, flushInterval          time.Duration
	reqRate, byteRate                float64
	order, keepSet, keepLog, dropLog string
	orderOverhead, keepMaxSize       uint64
}

// main runs the command and exits with a code that tells a hard stop and a
// clean stop apart from a failure.
func main() {
	// Declare the command and its flags.
	a := &args{}
	app := cli.NewApp()
	app.Name = "spacewave-world-rescue"
	app.Usage = "fetch listed blocks from PackStore packfiles on an FTPS server into a local pack set"
	app.Flags = []cli.Flag{
		&cli.StringFlag{Name: "refs", Usage: "list of wanted blocks, one \"<store> <hex key> [...]\" per line as block-pass --input takes; gzip allowed", Destination: &a.refs, Required: true},
		&cli.StringFlag{Name: "index", Usage: "copy of the writer's pack index SQLite file, opened read-only", Destination: &a.index, Required: true},
		&cli.StringFlag{Name: "index-table", Usage: "table of the pack index file", Value: defaultIndexTable, Destination: &a.indexTable},
		&cli.StringFlag{Name: "index-scratch", Usage: "directory on the index's file system for the read-only view's links and -shm; default rescue-index-view beside the index", Destination: &a.indexScratch},
		&cli.StringFlag{Name: "store-id", Usage: "block store id the index rows are keyed by", Destination: &a.storeID, Required: true},
		&cli.StringFlag{Name: "addr", Usage: "FTPS server host:port", Destination: &a.addr, Required: true},
		&cli.StringFlag{Name: "netrc", Usage: "netrc file with the server's login", Destination: &a.netrc, Required: true},
		&cli.StringFlag{Name: "root", Usage: "server directory holding the prefix", Value: "hyperline-packs", Destination: &a.root},
		&cli.StringFlag{Name: "prefix", Usage: "PackStore key prefix; packs are {root}/{prefix}packs/{id}", Value: "world/packs/", Destination: &a.prefix},
		&cli.StringFlag{Name: "out", Usage: "output directory of the pack set", Destination: &a.out, Required: true},
		&cli.StringFlag{Name: "layout", Usage: "output layout: packstore writes packs/{id} and entries/{id}; adopt writes {id} and {id}.entry flat, as hyperline-backend world-adopt-packs reads", Value: layoutPackStore, Destination: &a.layout},
		&cli.StringFlag{Name: "ledger", Usage: "ledger file; default {out}/rescue.ledger", Destination: &a.ledgerPath},
		&cli.IntFlag{Name: "sessions", Usage: "FTPS sessions at most, 1 to 9", Value: 4, Destination: &a.sessions},
		&cli.IntFlag{Name: "min-wanted", Usage: "take only packs holding more than this many wanted blocks; 256 is tier 1", Destination: &a.minWanted},
		&cli.BoolFlag{Name: "ranges", Usage: "read by block ranges a pack whose wanted blocks all have extents in the index, when that costs fewer bytes than the whole pack", Destination: &a.ranges},
		&cli.Uint64Flag{Name: "range-overhead", Usage: "bytes a request costs beyond its body, for --ranges", Value: 4096, Destination: &a.overhead},
		&cli.Uint64Flag{Name: "max-bytes", Usage: "most bytes fetched across runs; 0 refuses to run", Destination: &a.maxBytes},
		&cli.Uint64Flag{Name: "max-pack-mib", Usage: "largest pack read whole, in MiB; a larger one is skipped and its refs recorded too-large", Value: 1024, Destination: &a.maxPackMiB},
		&cli.Float64Flag{Name: "disk-floor-gb", Usage: "stop cleanly when the output disk has less free", Value: 20, Destination: &a.diskFloorGB},
		&cli.DurationFlag{Name: "timeout", Usage: "per-request timeout", Value: 2 * time.Minute, Destination: &a.timeout},
		&cli.IntFlag{Name: "retries", Usage: "retries of a failed request, never of a 530, 421 or session limit", Value: 3, Destination: &a.retries},
		&cli.DurationFlag{Name: "backoff", Usage: "first retry wait, doubling", Value: 2 * time.Second, Destination: &a.backoff},
		&cli.DurationFlag{Name: "max-backoff", Usage: "longest retry wait", Value: time.Minute, Destination: &a.maxBackoff},
		&cli.Float64Flag{Name: "requests-per-sec", Usage: "request rate cap across sessions; 0 is none", Value: 4, Destination: &a.reqRate},
		&cli.Float64Flag{Name: "bytes-per-sec", Usage: "byte rate cap across sessions; 0 is none", Value: 20 << 20, Destination: &a.byteRate},
		&cli.BoolFlag{Name: "retry-failed", Usage: "fetch again the refs and packs the ledger records as failed", Destination: &a.retryFailed},
		&cli.DurationFlag{Name: "flush-interval", Usage: "longest wait of verified blocks for their write to the output and their ref lines; each flush closes a synthetic pack. The byte total is synced after every pack", Value: 10 * time.Minute, Destination: &a.flushInterval},
		&cli.StringFlag{Name: "order", Usage: "pack order: wanted (most wanted refs first) or keys-per-time (wanted refs per pack size plus --order-overhead, most first)", Value: orderWanted, Destination: &a.order},
		&cli.Uint64Flag{Name: "order-overhead", Usage: "bytes a pack's request time is worth, for --order keys-per-time: the per-pack time times the session byte rate", Value: 10_304_340, Destination: &a.orderOverhead},
		&cli.StringFlag{Name: "packs", Usage: "file of pack ids, the first field of each line: plan only these packs, leaving the other packs' refs for a later run, so a restart takes the same packs", Destination: &a.packList},
		&cli.BoolFlag{Name: "stop-on-drop", Usage: "stop the run when a connection drops without a server answer, with no redial and no new session, logging \"stop: connection dropped\"", Destination: &a.stopOnDrop},
		&cli.IntFlag{Name: "max-dials", Usage: "most FTPS sessions dialed in the run, redials included; past it a dial is refused and the run stops; 0 is no limit", Destination: &a.maxDials},
		&cli.IntFlag{Name: "max-packs", Usage: "take only the first this many packs in --order, leaving the rest's refs for a later run; 0 takes all", Destination: &a.maxPacks},
		&cli.StringFlag{Name: "keep-set", Usage: "list of hex block keys or 16-byte tails: blocks of a whole pack kept beside the wanted, verified against their keys; whole packs only", Destination: &a.keepSet},
		&cli.Uint64Flag{Name: "keep-max-size", Usage: "drop a keep-set block larger than this many bytes, listing it in --drop-log; 0 is no cut", Destination: &a.keepMaxSize},
		&cli.StringFlag{Name: "keep-log", Usage: "file appended one \"<pack id> <hex key> <size>\" line per keep-set block stored; default {out}/kept.tsv", Destination: &a.keepLog},
		&cli.StringFlag{Name: "drop-log", Usage: "file appended one \"<pack id> <hex key> <offset> <size>\" line per keep-set block past --keep-max-size; default {out}/dropped.tsv", Destination: &a.dropLog},
		&cli.DurationFlag{Name: "progress", Usage: "time between progress lines", Value: 30 * time.Second, Destination: &a.progress},
	}
	app.Action = func(*cli.Context) error { return a.run() }

	// Run it and exit by the kind of its error.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		var hard *hardStopError
		if errors.As(err, &hard) {
			os.Exit(exitHardStop)
		}
		var stopped stoppedError
		if errors.As(err, &stopped) {
			os.Exit(exitStopped)
		}
		os.Exit(1)
	}
}

// stoppedError reports a clean early stop.
type stoppedError string

func (e stoppedError) Error() string { return "stopped cleanly: " + string(e) }

// run checks the flags, opens the index and the server, and runs the rescue.
func (a *args) run() error {
	// Refuse limits that would run unbounded or past the server's caps,
	// before reading any credential.
	if a.maxBytes == 0 {
		return errors.New("--max-bytes is 0: set a byte cap to run")
	}
	if a.sessions < 1 || a.sessions > maxSessions {
		return errors.New("--sessions must be 1 to 9")
	}
	if a.maxPackMiB == 0 {
		return errors.New("--max-pack-mib is 0: set a pack size limit to run")
	}
	if a.minWanted < 0 {
		return errors.New("--min-wanted must not be negative")
	}

	// Refuse an unknown output layout or pack order, or a negative pack cap.
	if a.layout != layoutPackStore && a.layout != layoutAdopt {
		return errors.New("--layout must be packstore or adopt")
	}
	if a.maxPacks < 0 || a.maxDials < 0 {
		return errors.New("--max-packs and --max-dials must not be negative")
	}

	// Read the pack list that bounds the plan.
	var packs map[string]bool
	if a.packList != "" {
		var err error
		if packs, err = readPackList(a.packList); err != nil {
			return err
		}
	}
	if a.order != orderWanted && a.order != orderTime {
		return errors.New("--order must be wanted or keys-per-time")
	}

	// Read the keep set and open its kept and dropped logs, whole packs only.
	var keep keepOptions
	if a.keepSet != "" {
		if a.ranges {
			return errors.New("--keep-set reads whole packs: drop --ranges")
		}
		set, err := readKeepSet(a.keepSet)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(a.out, 0o755); err != nil {
			return err
		}
		kept, err := os.OpenFile(cmp.Or(a.keepLog, filepath.Join(a.out, "kept.tsv")), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer kept.Close()
		dropped, err := os.OpenFile(cmp.Or(a.dropLog, filepath.Join(a.out, "dropped.tsv")), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer dropped.Close()
		keep = keepOptions{Set: set, MaxSize: a.keepMaxSize, Kept: kept, Dropped: dropped}
	}

	// Stop cleanly on an interrupt or a TERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Open the index copy read-only through a view that leaves its files,
	// -wal and -shm included, as they are.
	if _, err := os.Stat(a.index); err != nil {
		return err
	}
	store, err := openIndexView(ctx, a.index, cmp.Or(a.indexScratch, filepath.Join(filepath.Dir(a.index), "rescue-index-view")), a.indexTable)
	if err != nil {
		return err
	}
	defer store.Close()
	index := kvtx_prefixer.NewPrefixer(store, block_store_s3.PackIndexPrefix(a.storeID))

	// Log in from the netrc. The logins are never printed.
	host := a.addr
	if h, _, err := net.SplitHostPort(a.addr); err == nil {
		host = h
	}
	logins, err := block_store_ftps.ReadNetrc(a.netrc, host)
	if err != nil {
		return err
	}
	client := block_store_ftps.NewClient(block_store_ftps.Config{
		Addr:          a.addr,
		Logins:        logins,
		MaxSessions:   a.sessions,
		ConnsPerLogin: a.sessions,
		NoFullRetry:   true,
		CloseOnDone:   true,
		MaxDials:      a.maxDials,
	})
	defer client.Close()

	// Run.
	if err := os.MkdirAll(a.out, 0o755); err != nil {
		return err
	}
	res, err := run(ctx, &options{
		Bucket:        a.root,
		Prefix:        a.prefix,
		Index:         index,
		RefList:       a.refs,
		Ledger:        cmp.Or(a.ledgerPath, filepath.Join(a.out, "rescue.ledger")),
		OutDir:        a.out,
		Layout:        a.layout,
		Sessions:      a.sessions,
		Plan:          planOptions{Ranges: a.ranges, Overhead: a.overhead},
		MinWanted:     a.minWanted,
		Order:         a.order,
		MaxPacks:      a.maxPacks,
		Packs:         packs,
		OrderOverhead: a.orderOverhead,
		Keep:          keep,
		MaxPackBytes:  a.maxPackMiB << 20,
		Fetch: fetchOptions{
			Timeout: a.timeout, Retries: a.retries, Backoff: a.backoff, MaxBackoff: a.maxBackoff,
			RequestsPerSec: a.reqRate, BytesPerSec: a.byteRate, CapBytes: a.maxBytes,
			StopOnDrop: a.stopOnDrop,
		},
		DiskFloorBytes: uint64(a.diskFloorGB * (1 << 30)),
		Progress:       a.progress,
		FlushInterval:  a.flushInterval,
		RetryFailed:    a.retryFailed,
		Log:            os.Stderr,
	}, client)
	if err != nil {
		return err
	}
	if res.Stop != "" {
		return stoppedError(res.Stop)
	}
	return nil
}

// readPackList reads a file of pack ids, the first field of each non-empty
// line, and fails on an empty list.
func readPackList(path string) (map[string]bool, error) {
	// Read the file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Take the first field of each line.
	packs := make(map[string]bool)
	for line := range strings.Lines(string(data)) {
		if f := strings.Fields(line); len(f) != 0 {
			packs[f[0]] = true
		}
	}
	if len(packs) == 0 {
		return nil, errors.New(path + ": no pack ids")
	}
	return packs, nil
}

// Command world-punch releases the disk blocks of a closed SQLite database's
// freelist leaf pages with fallocate(PUNCH_HOLE|KEEP_SIZE). The file size,
// page numbers and every in-use and trunk page are unchanged; SQLite reads
// a free leaf without its content, so a hole reads as the zero page it
// would write itself with secure_delete.
//
// The default is a dry run. -range-sha punches, and only when a fresh walk
// matches the hash a dry run printed. -live reads a running database
// read-only, takes no locks and cannot punch.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/sqlitefree"
)

const wantPageSize = 4096

type opts struct {
	path     string
	reserve  int64
	rangeSHA string
	live     bool
	cross    bool
	mbps     float64
	mbpsFile string
	readers  int
	scan     bool
	streams  int
	parsers  int
	probeLim time.Duration
}

func main() {
	// Define the flags.
	var o opts
	flag.Int64Var(&o.reserve, "reserve", 1<<30, "bytes of leaves to keep unpunched, taken whole trunks from the head of the chain")
	flag.StringVar(&o.rangeSHA, "range-sha", "", "punch, if a fresh walk gives this range sha256 from a dry run")
	flag.BoolVar(&o.live, "live", false, "read a running database read-only: estimates only, no locks, cannot punch")
	flag.BoolVar(&o.cross, "crosscheck", false, "read-only: walk every b-tree and overflow chain and check none of their pages is on the freelist; no locks, cannot punch")
	flag.Float64Var(&o.mbps, "mbps", 100, "-crosscheck read cap in MB/s, 0 for none; -scan defaults to none")
	flag.StringVar(&o.mbpsFile, "mbps-file", "", "-crosscheck: reread every second; a number in it replaces the -mbps cap")
	flag.IntVar(&o.readers, "readers", 8, "-crosscheck parallel readers (reads in flight), 1 to 1024")

	// Define the -scan flags.
	flag.BoolVar(&o.scan, "scan", false, "-crosscheck of a stopped file: walk interior pages, then read the file once in page order; refuses a nonempty -wal")
	flag.IntVar(&o.streams, "streams", 32, "-scan parallel sequential streams of 1 MiB reads, 1 to 1024")
	flag.IntVar(&o.parsers, "parsers", runtime.NumCPU(), "-scan parse workers fed by the streams through a bounded queue")
	flag.DurationVar(&o.probeLim, "probe-limit", 0, "-scan: a minute into the sequential pass, stop (exit 4) if its projected time over the whole file exceeds this; 0 for no probe")

	// Parse them and take the one database path.
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: world-punch [-live | -crosscheck [-mbps N] [-mbps-file PATH] | -range-sha HASH] [-reserve BYTES] DB")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	o.path = flag.Arg(0)

	// A scan runs on a stopped file, so it reads uncapped unless -mbps
	// was given.
	mbpsSet := false
	flag.Visit(func(f *flag.Flag) { mbpsSet = mbpsSet || f.Name == "mbps" })
	if o.scan && !mbpsSet {
		o.mbps = 0
	}

	// Run the chosen mode; crosscheck exits 3 on FAIL and 4 when stopped
	// by SIGTERM or SIGINT.
	var err error
	switch {
	case (o.live || o.cross) && o.rangeSHA != "":
		err = errors.New("-live and -crosscheck cannot punch: drop -range-sha")
	case o.live && o.cross:
		err = errors.New("pick one of -live and -crosscheck")
	case o.cross && (o.readers < 1 || o.readers > 1024):
		err = errors.Errorf("-readers %d: want 1 to 1024", o.readers)
	case o.scan && !o.cross:
		err = errors.New("-scan needs -crosscheck")
	case o.scan && (o.streams < 1 || o.streams > 1024):
		err = errors.Errorf("-streams %d: want 1 to 1024", o.streams)
	case o.cross:
		var code int
		code, err = runCrosscheck(os.Stdout, o, interruptOnSignal())
		if err == nil && code != 0 {
			os.Exit(code)
		}
	case o.live:
		err = runLive(os.Stdout, o)
	default:
		err = runLocked(os.Stdout, o)
	}

	// Report an error.
	if err != nil {
		fmt.Fprintln(os.Stderr, "world-punch:", err)
		os.Exit(1)
	}
}

// interruptOnSignal returns a channel closed at the first SIGTERM or
// SIGINT, so a stopped crosscheck can print what it found.
func interruptOnSignal() <-chan struct{} {
	// Catch the stop signals.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	// Close stop at the first signal.
	stop := make(chan struct{})
	go func() {
		s := <-sig
		fmt.Fprintf(os.Stdout, "signal %s %s: stopping the walk for a partial report\n", s, time.Now().UTC().Format("15:04:05Z"))
		close(stop)
	}()
	return stop
}

// runLive reports estimates from a file another process may be writing. It
// opens the file read-only, so fallocate could not succeed on it, and it
// never calls punch.
func runLive(out io.Writer, o opts) error {
	// Open the file read-only.
	f, err := os.Open(o.path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read the header, going on past a warning.
	st, err := f.Stat()
	if err != nil {
		return err
	}
	h, herr := readHeader(f, st.Size())
	if h.PageSize == 0 {
		return herr
	}

	// Announce the mode and any header warning.
	fmt.Fprintln(out, "LIVE read-only estimate: no lock held, the WAL is not read, the freelist may change while it is walked")
	if herr != nil {
		fmt.Fprintln(out, "header warning:", herr)
	}

	// Walk tolerantly and report the plan and any tear.
	w, _ := walkFreelist(f, h, true)
	p := makePlan(w, h, o.reserve)
	report(out, h, w, p, st)
	if w.Torn != nil {
		fmt.Fprintln(out, "walk stopped early (torn or changing chain):", w.Torn)
	}
	return nil
}

// runCrosscheck walks a possibly running file read-only. It returns exit
// code 0 on PASS, 3 on FAIL (see failReasons) and 4 when interrupt closed
// before the check finished; then it prints a partial report.
func runCrosscheck(out io.Writer, o opts, interrupt <-chan struct{}) (int, error) {
	// A scan reads only the main file, so it refuses a WAL with frames or
	// a hot rollback journal.
	if o.scan {
		if err := checkNoJournal(o.path); err != nil {
			return 0, errors.Wrap(err, "-scan is for stopped files")
		}
	}

	// Open the file read-only and read its header.
	f, err := os.Open(o.path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	h, herr := readHeader(f, st.Size())
	if h.PageSize == 0 || (o.scan && herr != nil) {
		return 0, herr // a scan needs file size = header pages x page size
	}

	// Announce the mode and any header warning.
	fmt.Fprintln(out, "CROSSCHECK read-only: no lock held, the WAL is not read, a live file may change while it is walked")
	if herr != nil {
		fmt.Fprintln(out, "header warning:", herr)
	}

	// Serve tree reads from O_DIRECT when the file system allows it.
	co := ccOpts{readers: o.readers, mbps: o.mbps, mbpsFile: o.mbpsFile, retries: 3, retryIn: 5 * time.Second, every: time.Minute, snapEvery: 10 * time.Second, out: out, interrupt: interrupt, streams: o.streams, parsers: o.parsers, probeLimit: o.probeLim}
	if d, err := openDirect(o.path); err == nil {
		defer d.Close()
		co.bulk = d
		fmt.Fprintln(out, "tree reads use O_DIRECT")
	} else {
		fmt.Fprintln(out, "tree reads use the page cache:", err)
	}

	// Walk or scan, printing the scan's phase lines.
	check := crosscheck
	if o.scan {
		fmt.Fprintln(out, "SCAN: interior walk with", o.readers, "readers, then one sequential pass with", o.streams, "streams and", o.parsers, "parse workers; read cap", mbpsCap(o.mbps, o.mbpsFile))
		check = scanCheck
	}
	res, err := check(f, h, co)
	if err != nil {
		return 0, err
	}
	if o.scan {
		res.printScan(out)
	}

	// Report partially if interrupted, else in full.
	if res.interrupted {
		res.printPartial(out)
		return 4, nil
	}
	res.print(out)
	if res.failed() {
		return 3, nil
	}
	return 0, nil
}

// runLocked is the dry run, and the punch when o.rangeSHA is set.
func runLocked(out io.Writer, o opts) error {
	// Open the file and lock out every SQLite connection.
	f, err := os.OpenFile(o.path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	release, err := lockExclusive(f, o.path)
	if err != nil {
		return err
	}
	defer release()

	// Refuse a journal or WAL with pages, and a header the punch does not
	// suit.
	if err := checkNoJournal(o.path); err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := readHeader(f, st.Size())
	if err != nil {
		return err
	}
	if h.PageSize != wantPageSize {
		return errors.Errorf("page_size is %d, want %d", h.PageSize, wantPageSize)
	}
	if err := sqlitefree.CheckAutoVacuum(h); err != nil {
		return err
	}

	// Walk strictly and report the plan, stopping for a dry run or a
	// changed freelist.
	w, p, err := strictPlan(f, h, o.reserve)
	if err != nil {
		return err
	}
	report(out, h, w, p, st)
	if o.rangeSHA == "" {
		fmt.Fprintln(out, "dry run: nothing punched. To punch, rerun with -range-sha", p.SHA)
		return nil
	}
	if o.rangeSHA != p.SHA {
		return errors.Errorf("range sha256 is %s, -range-sha is %s: the freelist changed since the dry run", p.SHA, o.rangeSHA)
	}

	// Punch and flush the file.
	if err := punchRanges(f, h, p.Ranges); err != nil {
		return errors.Wrap(err, "punch (the file stays valid: every punched page was a free leaf)")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "fsync")
	}

	// Walk again and require the same plan.
	_, after, err := strictPlan(f, h, o.reserve)
	if err != nil {
		return errors.Wrap(err, "walk after punch")
	}
	if after.SHA != p.SHA {
		return errors.Errorf("range sha256 after punch is %s, before %s", after.SHA, p.SHA)
	}

	// Report what was punched and the allocation before and after.
	st2, err := f.Stat()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "punched %d ranges, %d bytes; walk after punch matches %s\n", len(p.Ranges), int64(p.PunchPages)*h.PageSize, p.SHA)
	fmt.Fprintf(out, "allocated bytes: before %d, after %d\n", allocated(st), allocated(st2))
	return nil
}

// checkNoJournal refuses unless the WAL is absent or empty and there is no
// rollback journal, so the main file holds every committed page.
func checkNoJournal(path string) error {
	for _, suf := range []string{"-wal", "-journal"} {
		st, err := os.Stat(path + suf)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Size() != 0 {
			return errors.Errorf("%s%s is %d bytes: checkpoint it (wal_checkpoint(TRUNCATE)) and stop every writer first", path, suf, st.Size())
		}
	}
	return nil
}

// report prints the header, the walk and the plan.
func report(out io.Writer, h header, w *walk, p plan, st os.FileInfo) {
	// Print the header and how the walk compares with its freelist count.
	fmt.Fprintf(out, "page size %d, pages %d, auto_vacuum %v\n", h.PageSize, h.DBPages, h.AutoVac)
	got := uint64(len(w.Trunks)) + uint64(len(w.Leaves))
	fmt.Fprintf(out, "header freelist count %d; walked trunks %d + leaves %d = %d (match %v)\n", h.FreeCount, len(w.Trunks), len(w.Leaves), got, got == uint64(h.FreeCount))

	// Print the plan and the file's allocation.
	fmt.Fprintf(out, "kept back: leaves of the first %d trunks, %d pages, %d bytes\n", p.KeptTrunks, p.KeptPages, int64(p.KeptPages)*h.PageSize)
	fmt.Fprintf(out, "to release: %d ranges, %d pages, %d bytes\n", len(p.Ranges), p.PunchPages, int64(p.PunchPages)*h.PageSize)
	fmt.Fprintf(out, "estimate: ext4 extents up to +%d, extent tree up to +%d bytes\n", len(p.Ranges), int64(len(p.Ranges)+339)/340*4096)
	fmt.Fprintf(out, "allocated bytes now %d of size %d\n", allocated(st), st.Size())
	fmt.Fprintf(out, "range sha256 %s\n", p.SHA)
}

// mbpsCap describes the -crosscheck read cap.
func mbpsCap(mbps float64, file string) string {
	c := "none"
	if mbps > 0 {
		c = fmt.Sprintf("%g MB/s", mbps)
	}
	if file != "" {
		c += " (-mbps-file " + file + " may replace it)"
	}
	return c
}

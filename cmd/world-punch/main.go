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
	"time"

	"github.com/pkg/errors"
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
}

func main() {
	var o opts
	flag.Int64Var(&o.reserve, "reserve", 1<<30, "bytes of leaves to keep unpunched, taken whole trunks from the head of the chain")
	flag.StringVar(&o.rangeSHA, "range-sha", "", "punch, if a fresh walk gives this range sha256 from a dry run")
	flag.BoolVar(&o.live, "live", false, "read a running database read-only: estimates only, no locks, cannot punch")
	flag.BoolVar(&o.cross, "crosscheck", false, "read-only: walk every b-tree and overflow chain and check none of their pages is on the freelist; no locks, cannot punch")
	flag.Float64Var(&o.mbps, "mbps", 100, "-crosscheck read cap in MB/s, 0 for none")
	flag.StringVar(&o.mbpsFile, "mbps-file", "", "-crosscheck: reread every second; a number in it replaces the -mbps cap")
	flag.IntVar(&o.readers, "readers", 8, "-crosscheck parallel readers")
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
	var err error
	switch {
	case (o.live || o.cross) && o.rangeSHA != "":
		err = errors.New("-live and -crosscheck cannot punch: drop -range-sha")
	case o.live && o.cross:
		err = errors.New("pick one of -live and -crosscheck")
	case o.cross:
		var bad bool
		bad, err = runCrosscheck(os.Stdout, o)
		if err == nil && bad {
			os.Exit(3)
		}
	case o.live:
		err = runLive(os.Stdout, o)
	default:
		err = runLocked(os.Stdout, o)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "world-punch:", err)
		os.Exit(1)
	}
}

// runLive reports estimates from a file another process may be writing. It
// opens the file read-only, so fallocate could not succeed on it, and it
// never calls punch.
func runLive(out io.Writer, o opts) error {
	f, err := os.Open(o.path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	h, herr := readHeader(f, st.Size())
	if h.pageSize == 0 {
		return herr
	}
	fmt.Fprintln(out, "LIVE read-only estimate: no lock held, the WAL is not read, the freelist may change while it is walked")
	if herr != nil {
		fmt.Fprintln(out, "header warning:", herr)
	}
	w, _ := walkFreelist(f, h, true)
	p := makePlan(w, h, o.reserve)
	report(out, h, w, p, st)
	if w.torn != nil {
		fmt.Fprintln(out, "walk stopped early (torn or changing chain):", w.torn)
	}
	return nil
}

// runCrosscheck walks a possibly running file read-only. It reports true
// when an unexplained conflict remains.
func runCrosscheck(out io.Writer, o opts) (bool, error) {
	f, err := os.Open(o.path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	h, herr := readHeader(f, st.Size())
	if h.pageSize == 0 {
		return false, herr
	}
	fmt.Fprintln(out, "CROSSCHECK read-only: no lock held, the WAL is not read, a live file may change while it is walked")
	if herr != nil {
		fmt.Fprintln(out, "header warning:", herr)
	}
	co := ccOpts{readers: o.readers, mbps: o.mbps, mbpsFile: o.mbpsFile, retries: 3, retryIn: 5 * time.Second, every: time.Minute, snapEvery: 10 * time.Second, out: out}
	if d, err := openDirect(o.path); err == nil {
		defer d.Close()
		co.bulk = d
		fmt.Fprintln(out, "tree reads use O_DIRECT")
	} else {
		fmt.Fprintln(out, "tree reads use the page cache:", err)
	}
	res, err := crosscheck(f, h, co)
	if err != nil {
		return false, err
	}
	res.print(out)
	return res.failed(), nil
}

// runLocked is the dry run, and the punch when o.rangeSHA is set.
func runLocked(out io.Writer, o opts) error {
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
	if h.pageSize != wantPageSize {
		return errors.Errorf("page_size is %d, want %d", h.pageSize, wantPageSize)
	}
	if h.autoVac {
		return errors.New("auto_vacuum is not NONE (header offset 52 is nonzero): ptrmap pages track free pages, refusing")
	}
	w, p, err := strictPlan(f, h, o.reserve)
	if err != nil {
		return err
	}
	report(out, h, w, p, st)
	if o.rangeSHA == "" {
		fmt.Fprintln(out, "dry run: nothing punched. To punch, rerun with -range-sha", p.sha)
		return nil
	}
	if o.rangeSHA != p.sha {
		return errors.Errorf("range sha256 is %s, -range-sha is %s: the freelist changed since the dry run", p.sha, o.rangeSHA)
	}
	if err := punchRanges(f, h, p.ranges); err != nil {
		return errors.Wrap(err, "punch (the file stays valid: every punched page was a free leaf)")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "fsync")
	}
	_, after, err := strictPlan(f, h, o.reserve)
	if err != nil {
		return errors.Wrap(err, "walk after punch")
	}
	if after.sha != p.sha {
		return errors.Errorf("range sha256 after punch is %s, before %s", after.sha, p.sha)
	}
	st2, err := f.Stat()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "punched %d ranges, %d bytes; walk after punch matches %s\n", len(p.ranges), int64(p.punchPages)*h.pageSize, p.sha)
	fmt.Fprintf(out, "allocated bytes: before %d, after %d\n", allocated(st), allocated(st2))
	return nil
}

// strictPlan walks the chain and refuses unless trunks plus leaves equals
// the header's freelist count.
func strictPlan(f io.ReaderAt, h header, reserve int64) (*walk, plan, error) {
	w, err := walkFreelist(f, h, false)
	if err != nil {
		return nil, plan{}, err
	}
	if got := uint64(len(w.trunks)) + uint64(len(w.leaves)); got != uint64(h.freeCount) {
		return nil, plan{}, errors.Errorf("trunks %d + leaves %d = %d, header freelist count is %d", len(w.trunks), len(w.leaves), got, h.freeCount)
	}
	return w, makePlan(w, h, reserve), nil
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

func report(out io.Writer, h header, w *walk, p plan, st os.FileInfo) {
	fmt.Fprintf(out, "page size %d, pages %d, auto_vacuum %v\n", h.pageSize, h.dbPages, h.autoVac)
	got := uint64(len(w.trunks)) + uint64(len(w.leaves))
	fmt.Fprintf(out, "header freelist count %d; walked trunks %d + leaves %d = %d (match %v)\n", h.freeCount, len(w.trunks), len(w.leaves), got, got == uint64(h.freeCount))
	fmt.Fprintf(out, "kept back: leaves of the first %d trunks, %d pages, %d bytes\n", p.keptTrunks, p.keptPages, int64(p.keptPages)*h.pageSize)
	fmt.Fprintf(out, "to release: %d ranges, %d pages, %d bytes\n", len(p.ranges), p.punchPages, int64(p.punchPages)*h.pageSize)
	fmt.Fprintf(out, "estimate: ext4 extents up to +%d, extent tree up to +%d bytes\n", len(p.ranges), int64(len(p.ranges)+339)/340*4096)
	fmt.Fprintf(out, "allocated bytes now %d of size %d\n", allocated(st), st.Size())
	fmt.Fprintf(out, "range sha256 %s\n", p.sha)
}

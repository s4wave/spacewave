package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	mrand "math/rand/v2"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

// keptDigestExt is appended to the new file's path to name the digest file
// copy -kept-digest writes and verify -digest reads.
const keptDigestExt = ".keptdigest"

// keptDigestVersion is the digest file format.
const keptDigestVersion = 1

// sourceID names a source file: verify -digest refuses a digest file whose
// source differs from -world.
type sourceID struct {
	Size      int64
	PageSize  int64
	PageCount int64
	MaxRowid  int64
	Header    string
}

// keptDigest is the digest file: the rowSet of the rows a copy inserted,
// hashed with xxhash under two seeds kept here (a maphash seed cannot be
// saved), besides the evictable-bytes rows, whose values the copy rewrites
// and which are listed apart.
type keptDigest struct {
	Version    int
	Commit     string
	SeedA      uint64
	SeedB      uint64
	Rows       int64
	SumA       uint64
	SumB       uint64
	EvictBytes map[string]int64
	Kept       int64
	Dropped    int64
	Source     sourceID
	Written    time.Time
}

// xxRowHasher hashes rows for a rowSet as rowHasher does, with xxhash under
// saved seeds.
type xxRowHasher struct {
	a, b   *xxhash.Digest
	sa, sb uint64
}

// newXXRowHasher makes a hasher under the two seeds.
func newXXRowHasher(sa, sb uint64) *xxRowHasher {
	return &xxRowHasher{a: xxhash.NewWithSeed(sa), b: xxhash.NewWithSeed(sb), sa: sa, sb: sb}
}

// add adds the row of the fields to s, each length-prefixed.
func (h *xxRowHasher) add(s *rowSet, fields ...[]byte) {
	// Hash each field after its length under both seeds; Reset alone would
	// reset to seed 0.
	h.a.ResetWithSeed(h.sa)
	h.b.ResetWithSeed(h.sb)
	var l [8]byte
	for _, f := range fields {
		binary.LittleEndian.PutUint64(l[:], uint64(len(f)))
		_, _ = h.a.Write(l[:])
		_, _ = h.a.Write(f)
		_, _ = h.b.Write(l[:])
		_, _ = h.b.Write(f)
	}

	// Count the row into the set.
	s.n++
	s.a += h.a.Sum64()
	s.b += h.b.Sum64()
}

// buildCommit returns the vcs revision the binary was built from.
func buildCommit() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", ""
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					dirty = "+dirty"
				}
			}
		}
		if rev != "" {
			return rev + dirty
		}
	}
	return "unknown"
}

// readSourceID names the source file path, open as db.
func readSourceID(db *sql.DB, path, table string) (sourceID, error) {
	// The file's size and its header's page size and count.
	var id sourceID
	fi, err := os.Stat(path)
	if err != nil {
		return id, err
	}
	id.Size = fi.Size()
	h, err := readHeader(db)
	if err != nil {
		return id, err
	}
	id.PageSize, id.PageCount = h.pageSize, h.pageCount

	// The kv table's largest rowid.
	var mr sql.NullInt64
	if err := db.QueryRow("SELECT max(rowid) FROM " + table).Scan(&mr); err != nil {
		return id, err
	}
	id.MaxRowid = mr.Int64

	// The hash of the 100-byte database header, with its change counter.
	f, err := os.Open(path)
	if err != nil {
		return id, err
	}
	defer f.Close()
	var hdr [100]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return id, err
	}
	sum := sha256.Sum256(hdr[:])
	id.Header = hex.EncodeToString(sum[:])
	return id, nil
}

// newKeptDigest starts the digest of a copy, with fresh seeds.
func newKeptDigest(src sourceID) (*keptDigest, error) {
	// Draw the two hash seeds.
	var s [16]byte
	if _, err := rand.Read(s[:]); err != nil {
		return nil, err
	}

	// Name the binary and the source beside them.
	return &keptDigest{
		Version:    keptDigestVersion,
		Commit:     buildCommit(),
		SeedA:      binary.LittleEndian.Uint64(s[:8]),
		SeedB:      binary.LittleEndian.Uint64(s[8:]),
		EvictBytes: map[string]int64{},
		Source:     src,
	}, nil
}

// writeKeptDigest writes the digest file at path, whole or not at all: one
// "name value" line a field, and an "evictable-bytes <store> <n>" line a
// store.
func writeKeptDigest(path string, d *keptDigest) error {
	// Format the fields.
	var b strings.Builder
	fmt.Fprintf(&b, "version %d\ncommit %s\nseed_a %d\nseed_b %d\n", d.Version, d.Commit, d.SeedA, d.SeedB)
	fmt.Fprintf(&b, "rows %d\nsum_a %d\nsum_b %d\nkept %d\ndropped %d\n", d.Rows, d.SumA, d.SumB, d.Kept, d.Dropped)
	fmt.Fprintf(&b, "source_size %d\nsource_page_size %d\nsource_page_count %d\nsource_max_rowid %d\nsource_header_sha256 %s\n", d.Source.Size, d.Source.PageSize, d.Source.PageCount, d.Source.MaxRowid, d.Source.Header)
	fmt.Fprintf(&b, "written %s\n", d.Written.Format(time.RFC3339Nano))
	for _, obj := range sortedKeys(d.EvictBytes) {
		fmt.Fprintf(&b, "evictable-bytes %s %d\n", obj, d.EvictBytes[obj])
	}

	// Write a temporary file and rename it over path.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readKeptDigest reads the digest file at path.
func readKeptDigest(path string) (*keptDigest, error) {
	// Read the file.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse each line into its field.
	d := &keptDigest{EvictBytes: map[string]int64{}}
	ints := map[string]*int64{"rows": &d.Rows, "kept": &d.Kept, "dropped": &d.Dropped, "source_size": &d.Source.Size, "source_page_size": &d.Source.PageSize, "source_page_count": &d.Source.PageCount, "source_max_rowid": &d.Source.MaxRowid}
	uints := map[string]*uint64{"seed_a": &d.SeedA, "seed_b": &d.SeedB, "sum_a": &d.SumA, "sum_b": &d.SumB}
	for i, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if err := parseDigestLine(d, ints, uints, line); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
	}

	// Check its format.
	if d.Version != keptDigestVersion {
		return nil, fmt.Errorf("%s: version %d, want %d", path, d.Version, keptDigestVersion)
	}
	return d, nil
}

// parseDigestLine parses one line of a digest file into d.
func parseDigestLine(d *keptDigest, ints map[string]*int64, uints map[string]*uint64, line string) error {
	// Split the name from the value.
	name, val, ok := strings.Cut(line, " ")
	if !ok {
		return fmt.Errorf("%q: want a name and a value", line)
	}

	// Parse the value by the name.
	var err error
	switch {
	case ints[name] != nil:
		*ints[name], err = strconv.ParseInt(val, 10, 64)
	case uints[name] != nil:
		*uints[name], err = strconv.ParseUint(val, 10, 64)
	case name == "version":
		d.Version, err = strconv.Atoi(val)
	case name == "commit":
		d.Commit = val
	case name == "source_header_sha256":
		d.Source.Header = val
	case name == "written":
		d.Written, err = time.Parse(time.RFC3339Nano, val)
	case name == "evictable-bytes":
		obj, n, ok := strings.Cut(val, " ")
		if !ok {
			return fmt.Errorf("%q: want a store and a count", line)
		}
		d.EvictBytes[obj], err = strconv.ParseInt(n, 10, 64)
	default:
		return fmt.Errorf("%q: unknown field", name)
	}
	return err
}

// digestResult is what verify -digest found.
type digestResult struct {
	faults                []string
	rows                  int64
	spotKept, spotDropped int64
	spotAll               bool
	seed                  uint64
}

// fault records a fault, up to 40.
func (r *digestResult) fault(s string, a ...any) {
	if len(r.faults) < 40 {
		r.faults = append(r.faults, fmt.Sprintf(s, a...))
	}
}

// runVerifyDigest is verify -digest: verifyDigest, printed, failing on any
// fault.
func runVerifyDigest(o opts) error {
	// Check.
	res, err := verifyDigest(o)
	if err != nil {
		return err
	}

	// State what the check proves and what it does not.
	fmt.Println("verify -digest proves:")
	fmt.Println("  - the new file holds exactly the rows the copy inserted (count and two 64-bit sums of xxhash of every key and whole value, read raw from the new file)")
	fmt.Println("  - the new file's key index holds each table row's key and rowid once, in key order")
	fmt.Println("  - each evictable-bytes is the sum of the eviction entries the new file holds, and what the copy wrote")
	fmt.Println("  - a seeded random sample of source rows, decoded by SQLite (not the raw decoder), is in the new file byte for byte when the rule recomputed from the source's facts keeps it, and absent when it drops it")
	fmt.Println("verify -digest does NOT prove:")
	fmt.Println("  - a second, independent read of the source: a fault in the copy's raw read of the source (a wrong page or value read, which the digest would record as kept) is caught only if the sample hits it")
	fmt.Println("  - the verdict of every row: the drop rule is recomputed only for the sampled rows; a wrong drop outside the sample passes")

	// Report the counts and any fault.
	fmt.Printf("verify -digest: %d rows match the digest; spot check seed %d: %d kept and %d dropped source rows checked (all rows: %v)\n", res.rows, res.seed, res.spotKept, res.spotDropped, res.spotAll)
	if len(res.faults) != 0 {
		for _, s := range res.faults {
			fmt.Println("  FAULT", s)
		}
		return fmt.Errorf("verify -digest failed: %d faults", len(res.faults))
	}
	fmt.Println("verify -digest: ok")
	return nil
}

// verifyDigest reads the digest file of o.out, refuses it if its source is
// not o.world, reads the new file raw once to compare its rows with the
// digest and check evictable-bytes, checks the new file's key index, then
// spot checks o.spot kept and o.spot/10 dropped source rows through SQLite.
// An error is a failure to check; faults are what the check found.
func verifyDigest(o opts) (*digestResult, error) {
	// Read the digest file and open the source.
	start := time.Now()
	d, err := readKeptDigest(o.out + keptDigestExt)
	if err != nil {
		return nil, err
	}
	src, err := openRO(o.world)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	table, _, err := kvTable(src)
	if err != nil {
		return nil, err
	}

	// Refuse a digest file written for another source.
	id, err := readSourceID(src, o.world, table)
	if err != nil {
		return nil, err
	}
	if id != d.Source {
		return nil, fmt.Errorf("%s%s was written for another source: it names %+v, %s is %+v", o.out, keptDigestExt, d.Source, o.world, id)
	}
	if c := buildCommit(); c != d.Commit {
		fmt.Printf("note: the digest was written by commit %s, this binary is %s\n", d.Commit, c)
	}

	// Open the new file.
	dst, err := openRO(o.out)
	if err != nil {
		return nil, err
	}
	defer dst.Close()
	dtable, _, err := kvTable(dst)
	if err != nil {
		return nil, err
	}

	// The new file must be incremental auto_vacuum.
	res := &digestResult{}
	var av int64
	if err := dst.QueryRow("PRAGMA auto_vacuum").Scan(&av); err != nil {
		return nil, err
	}
	if av != 2 {
		res.fault("auto_vacuum is %d, want 2", av)
	}

	// Read the new file raw once and compare it with the digest.
	if err := checkNewRaw(o, dst, dtable, d, res); err != nil {
		return nil, err
	}
	fmt.Printf("verify -digest: new file read raw and its key index checked in %s\n", time.Since(start).Round(time.Second))

	// Spot check source rows through SQLite.
	t0 := time.Now()
	if err := spotCheck(o, src, dst, table, dtable, d, res); err != nil {
		return nil, err
	}
	fmt.Printf("verify -digest: spot check in %s, all checks in %s\n", time.Since(t0).Round(time.Second), time.Since(start).Round(time.Second))
	slices.Sort(res.faults)
	return res, nil
}

// newRawSums is what one raw read of the new file sums.
type newRawSums struct {
	mu sync.Mutex
	// got is the digest of the rows besides evictable-bytes, under the
	// digest file's seeds; idx is of each row's key and rowid.
	got, idx rowSet
	// bytes holds each evictable-bytes value; evict sums the entries.
	bytes map[string][]byte
	evict evictSums
}

// checkNewRaw reads the new file raw with o.readers goroutines and checks its
// rows against the digest, its evictable-bytes against its eviction entries
// and the copy's, and its key index against its table.
func checkNewRaw(o opts, dst *sql.DB, dtable string, d *keptDigest, res *digestResult) error {
	// Walk the new file's table b-tree.
	readers := max(o.readers, 1)
	dr, dleaves, err := rawTable(dst, o.out, readers)
	if err != nil {
		return err
	}
	defer dr.Close()

	// Read every row, each reader summing apart, then merging.
	sa, sb := maphash.MakeSeed(), maphash.MakeSeed()
	all := &newRawSums{bytes: map[string][]byte{}, evict: evictSums{}}
	err = dr.scanTable(dleaves, readers, nil, nil, func(int) (rawVisit, func() error) {
		return newRawVisitor(all, d, sa, sb)
	})
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	res.rows = all.got.n

	// The rows besides evictable-bytes are the ones the copy inserted.
	want := rowSet{n: d.Rows, a: d.SumA, b: d.SumB}
	if all.got.n != want.n {
		res.fault("digest: the new file holds %d rows besides evictable-bytes, the copy inserted %d", all.got.n, want.n)
	} else if all.got != want {
		res.fault("digest: the new file's %d rows differ from the %d the copy inserted (row digests differ)", all.got.n, want.n)
	}

	// Each evictable-bytes is what the copy wrote and the sum of the
	// entries the new file holds.
	checkEvictBytes(all, d, res)

	// The key index holds each table row's key and rowid, in strictly
	// increasing key order, as runVerifyRaw checks.
	idx, ordered, err := keyIndexSet(dst, dtable, sa, sb)
	if err != nil {
		return err
	}
	if !ordered {
		res.fault("the key index is not in strictly increasing key order")
	}
	if idx != all.idx {
		res.fault("the key index holds %d entries, the table %d rows, or their keys and rowids differ", idx.n, all.idx.n)
	}
	return nil
}

// newRawVisitor makes one reader's visitor of the new file's rows, and its
// flush, which merges its sums into all.
func newRawVisitor(all *newRawSums, d *keptDigest, sa, sb maphash.Seed) (rawVisit, func() error) {
	// The reader's own hashers and sums.
	xh := newXXRowHasher(d.SeedA, d.SeedB)
	ih := newRowHasher(sa, sb)
	var s, idx rowSet
	ws := evictSums{}
	var rid [8]byte

	// visit sums one row: its key and rowid, and its key and value but for
	// evictable-bytes, whose value is kept apart.
	visit := func(rowid int64, key []byte, _ int64, value func() ([]byte, error)) error {
		// Sum the key and rowid for the key index check.
		binary.BigEndian.PutUint64(rid[:], uint64(rowid))
		ih.add(&idx, key, rid[:])

		// Read the whole value.
		v, err := value()
		if err != nil {
			return fmt.Errorf("copy value of %q: %w", trunc(key), err)
		}

		// Keep evictable-bytes apart; sum every other row.
		if obj, rest, ok := objKey(key); ok && rest == evictBytesSeg {
			all.mu.Lock()
			all.bytes[obj] = v
			all.mu.Unlock()
			return nil
		}
		ws.add(key, v)
		xh.add(&s, key, v)
		return nil
	}

	// flush merges the reader's sums.
	flush := func() error {
		// Merge under the lock the readers share.
		all.mu.Lock()
		defer all.mu.Unlock()
		all.got.merge(s)
		all.idx.merge(idx)
		for k, n := range ws {
			all.evict[k] += n
		}
		return nil
	}
	return visit, flush
}

// checkEvictBytes checks each evictable-bytes of the new file is what the
// copy wrote and the sum of the new file's eviction entries.
func checkEvictBytes(all *newRawSums, d *keptDigest, res *digestResult) {
	// Each the copy wrote is in the new file as written.
	for _, obj := range sortedKeys(d.EvictBytes) {
		v, ok := all.bytes[obj]
		if !ok {
			res.fault("%s evictable-bytes missing from the new file", obj)
			continue
		}
		if w := strconv.FormatInt(d.EvictBytes[obj], 10); string(v) != w {
			res.fault("%s evictable-bytes is %q, the copy wrote %s", obj, v, w)
		}
	}

	// Each in the new file was written by the copy and sums its entries.
	for obj, v := range all.bytes {
		if _, ok := d.EvictBytes[obj]; !ok {
			res.fault("%s evictable-bytes in the new file, not written by the copy", obj)
		}
		if w := strconv.FormatInt(all.evict[obj], 10); string(v) != w {
			res.fault("%s evictable-bytes is %q, the new file's entries sum to %s", obj, v, w)
		}
	}
}

// keyIndexSet reads the key index of table through SQLite in key order and
// returns the rowSet of its keys and rowids, and whether the keys strictly
// increase.
func keyIndexSet(db *sql.DB, table string, sa, sb maphash.Seed) (rowSet, bool, error) {
	// Read the index alone, in key order.
	var idx rowSet
	h := newRowHasher(sa, sb)
	rows, err := db.Query("SELECT key, rowid FROM " + table + " INDEXED BY sqlite_autoindex_" + table + "_1 ORDER BY key")
	if err != nil {
		return idx, false, err
	}
	defer rows.Close()

	// Sum each entry and check the order.
	var last []byte
	var rid [8]byte
	ordered := true
	for rows.Next() {
		var key []byte
		var rowid int64
		if err := rows.Scan(&key, &rowid); err != nil {
			return idx, false, err
		}
		if idx.n > 0 && bytes.Compare(key, last) <= 0 {
			ordered = false
		}
		last = key
		binary.BigEndian.PutUint64(rid[:], uint64(rowid))
		h.add(&idx, key, rid[:])
	}
	return idx, ordered, rows.Err()
}

// spotChecker checks source rows against the new file through SQLite.
type spotChecker struct {
	f    *facts
	p    *dropPlan
	d    *keptDigest
	res  *digestResult
	sget *sql.Stmt
	dget *sql.Stmt
}

// check decides one source row by the drop rule and checks the new file: a
// kept row is there with the same value (evictable-bytes with the value the
// copy wrote), a dropped row is absent.
func (c *spotChecker) check(rowid int64, key []byte) error {
	// Decide the row by the rule, recomputed from the source's facts.
	dropped := false
	if bytes.HasPrefix(key, []byte(blockPrefix)) {
		dropped = c.f.blockVerdict(key) == vDrop
	} else if _, skip := c.p.skipEvict[string(key)]; skip {
		dg, ok := evictDigest(key)
		if !ok || c.f.verdict(dg) != vDrop {
			c.res.fault("spot: eviction entry %q left out, its block's verdict is not drop", trunc(key))
		}
		dropped = true
	}

	// Look the key up in the new file, by its key index.
	var dval []byte
	err := c.dget.QueryRow(key).Scan(&dval)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return err
	}

	// A dropped row is absent.
	if dropped {
		c.res.spotDropped++
		if !missing {
			c.res.fault("spot: %q is in the new file, the rule drops it", trunc(key))
		}
		return nil
	}

	// A kept row is present; evictable-bytes holds what the copy wrote.
	c.res.spotKept++
	if missing {
		c.res.fault("spot: %q missing from the new file (%s)", trunc(key), reason(c.f, key))
		return nil
	}
	if obj, rest, ok := objKey(key); ok && rest == evictBytesSeg {
		if w := strconv.FormatInt(c.d.EvictBytes[obj], 10); string(dval) != w {
			c.res.fault("spot: %s evictable-bytes is %q, the copy wrote %s", obj, dval, w)
		}
		return nil
	}

	// Any other kept row's value equals the source's, byte for byte.
	var sval []byte
	if err := c.sget.QueryRow(rowid).Scan(&sval); err != nil {
		return err
	}
	if !bytes.Equal(sval, dval) {
		c.res.fault("spot: value of %q differs (source %d bytes, new file %d)", trunc(key), len(sval), len(dval))
	}
	return nil
}

// spotCheck samples source rows by random rowid, seeded, and checks each
// with spotChecker.check. It stops once o.spot kept and o.spot/10 dropped
// rows are checked. When those cover the source's rows it checks every row
// instead.
func spotCheck(o opts, src, dst *sql.DB, table, dtable string, d *keptDigest, res *digestResult) error {
	// Open the pack index and read the source's table SQL.
	packs, err := openRO(o.packs)
	if err != nil {
		return err
	}
	defer packs.Close()
	_, stmt, err := kvTable(src)
	if err != nil {
		return err
	}

	// Recompute the drop rule's facts from the source, raw, as copy does.
	o.rawFacts = true
	f, err := loadFacts(o, src, table, packs)
	if err != nil {
		return err
	}
	present, err := blockDigestsRaw(o, src, o.world, table, stmt)
	if err != nil {
		return err
	}

	// Prepare the point reads of the source and the new file.
	c := &spotChecker{f: f, p: planDrops(f, present), d: d, res: res}
	if c.sget, err = src.Prepare("SELECT value FROM " + table + " WHERE rowid = ?"); err != nil {
		return err
	}
	defer c.sget.Close()
	if c.dget, err = dst.Prepare("SELECT value FROM " + dtable + " WHERE key = ?"); err != nil {
		return err
	}
	defer c.dget.Close()

	// Seed the sample, printing the seed.
	seed := o.seed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	res.seed = seed
	fmt.Printf("verify -digest: spot check seed %d (rerun with -seed %d)\n", seed, seed)

	// A sample as large as the source checks every row.
	wantKept, wantDrop := int64(o.spot), int64(o.spot/10)
	res.spotAll = wantKept+wantDrop >= d.Kept+d.Dropped
	if res.spotAll {
		return spotAllRows(src, table, c)
	}
	return spotSample(src, table, c, seed, wantKept, wantDrop)
}

// spotAllRows checks every source row, in rowid order.
func spotAllRows(src *sql.DB, table string, c *spotChecker) error {
	// List the rows first: check queries the same one-connection pool.
	type rk struct {
		rowid int64
		key   []byte
	}
	var all []rk
	rows, err := src.Query("SELECT rowid, key FROM " + table + " ORDER BY rowid")
	if err != nil {
		return err
	}
	for rows.Next() {
		var r rk
		if err := rows.Scan(&r.rowid, &r.key); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	// Check each.
	for _, r := range all {
		if err := c.check(r.rowid, r.key); err != nil {
			return err
		}
	}
	return nil
}

// spotSample checks distinct source rows found from random rowids, the
// first row at or after each, until wantKept kept and wantDrop dropped rows
// are checked or twenty times their sum is tried.
func spotSample(src *sql.DB, table string, c *spotChecker, seed uint64, wantKept, wantDrop int64) error {
	// Prepare the row lookup by rowid.
	next, err := src.Prepare("SELECT rowid, key FROM " + table + " WHERE rowid >= ? ORDER BY rowid LIMIT 1")
	if err != nil {
		return err
	}
	defer next.Close()

	// Draw rows until both counts are met.
	rng := mrand.New(mrand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	seen := map[int64]struct{}{}
	maxTries := 20 * (wantKept + wantDrop)
	for tries := int64(0); tries < maxTries && (c.res.spotKept < wantKept || c.res.spotDropped < wantDrop); tries++ {
		var rowid int64
		var key []byte
		err := next.QueryRow(1+rng.Int64N(c.d.Source.MaxRowid)).Scan(&rowid, &key)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, ok := seen[rowid]; ok {
			continue
		}
		seen[rowid] = struct{}{}
		if err := c.check(rowid, key); err != nil {
			return err
		}
	}

	// Say so when the tries ran out first.
	if c.res.spotKept < wantKept || c.res.spotDropped < wantDrop {
		fmt.Printf("note: spot check found %d kept and %d dropped rows of the %d and %d asked for in %d tries\n", c.res.spotKept, c.res.spotDropped, wantKept, wantDrop, maxTries)
	}
	return nil
}

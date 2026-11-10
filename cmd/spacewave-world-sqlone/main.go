//go:build !js && !wasip1 && !windows

// Command spacewave-world-sqlone reads a list of blocks from a block store's
// rows in a SQL blocks table over one read-only database session, verifies
// each body against its key, and writes the found blocks as block-export
// frames: the uvarint length of the key, the key, a uvarint 1 when the row
// records the refs and 0 when it does not, the uvarint length of the block
// object, and the object.
//
// It reads with the store's own batch query, at most --batch keys per query
// and --rate queries per second. An append-only ledger records each batch and
// the output sizes after it, so a stopped run resumes where it stopped. Any
// SQL error gets one reconnect after a minute, then the run stops; a refused,
// too-many or auth error stops it at once. It never writes the database and
// never prints the credentials it reads.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/s4wave/spacewave/db/block"
	block_store_sql "github.com/s4wave/spacewave/db/block/store/sql"
)

// keyPrefix starts every key: the marshaled hash.Hash of a SHA-256.
var keyPrefix = []byte{0x08, 0x01, 0x12, 0x20}

// exitStop is the exit code of a stop on an error.
const exitStop = 2

func main() {
	code, err := run()
	if err != nil {
		logf("error: %v", err)
	}
	os.Exit(code)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"), fmt.Sprintf(format, args...))
}

func run() (int, error) {
	// Declare the flags.
	keysPath := flag.String("keys", "", "file of hex keys, one per line (the last field of a line is used)")
	envPath := flag.String("env", "", "file of HYPERLINE_WORLD_SQL_* NAME=VALUE lines")
	storeID := flag.String("store-id", "", "block store id of the rows")
	outDir := flag.String("out", "", "output directory")
	name := flag.String("name", "sqlone", "base name of the output files")
	batch := flag.Int("batch", 500, "keys per query, at most 500")
	rate := flag.Float64("rate", 5, "queries per second, at most 5")
	stopFile := flag.String("stop-file", "", "stop cleanly when this file exists")

	// Parse them and require the inputs.
	flag.Parse()
	if *keysPath == "" || *envPath == "" || *storeID == "" || *outDir == "" {
		return 1, errors.New("--keys, --env, --store-id and --out are required")
	}

	// Bound the batch and the rate.
	if *batch < 1 || *batch > 500 {
		return 1, errors.New("--batch must be 1-500")
	}
	if *rate <= 0 || *rate > 5 {
		return 1, errors.New("--rate must be above 0 and at most 5")
	}

	// Read the keys.
	keys, err := readKeys(*keysPath)
	if err != nil {
		return 1, err
	}

	// Read the database location, for one connection.
	conf, err := readEnv(*envPath)
	if err != nil {
		return 1, err
	}
	conf.MaxConns = 1

	// Open the output and resume from its ledger.
	out, err := openOutput(*outDir, *name)
	if err != nil {
		return 1, err
	}
	defer out.close()

	// Stop when the ledger already holds every batch.
	nb := (len(keys) + *batch - 1) / *batch
	logf("keys %d batches %d resume at batch %d (found %d missed %d hashbad %d)", len(keys), nb, out.next, out.found, out.missed, out.hashBad)
	if out.next >= nb {
		logf("done: found %d missed %d hashbad %d bytes %d", out.found, out.missed, out.hashBad, out.bytes)
		return 0, nil
	}

	// Open the one session.
	ctx := context.Background()
	r := &reader{conf: conf, table: "`" + strings.ReplaceAll(conf.Table, "`", "``") + "`", storeID: []byte(*storeID)}
	if err := r.connect(ctx); err != nil {
		return exitStop, err
	}
	defer r.close()

	// Read the batches in order at the rate, reconnecting once on an error.
	interval := time.Duration(float64(time.Second) / *rate)
	last := time.Time{}
	lastLog := time.Now()
	reconnected := false
	for b := out.next; b < nb; b++ {
		// Stop cleanly on the stop file.
		if *stopFile != "" {
			if _, err := os.Stat(*stopFile); err == nil {
				logf("stop file present, stopping at batch %d", b)
				return 3, nil
			}
		}

		// Wait out the rate, then query the batch.
		if wait := interval - time.Since(last); wait > 0 {
			time.Sleep(wait)
		}
		last = time.Now()
		chunk := keys[b**batch : min((b+1)**batch, len(keys))]
		rows, ms, err := r.read(ctx, chunk)

		// Stop on a fatal or second error; otherwise reconnect once and retry.
		if err != nil {
			if fatal(err) || reconnected {
				return exitStop, fmt.Errorf("batch %d: %w", b, err)
			}
			logf("batch %d: %v; one reconnect in 60 s", b, err)
			r.close()
			time.Sleep(time.Minute)
			reconnected = true
			if err := r.connect(ctx); err != nil {
				return exitStop, err
			}
			b--
			continue
		}

		// Record the batch, then report progress.
		if err := out.write(b, chunk, rows); err != nil {
			return 1, err
		}
		if b == out.first || time.Since(lastLog) >= 30*time.Second || b == nb-1 {
			lastLog = time.Now()
			logf("batch %d/%d query_ms %d found %d missed %d hashbad %d bytes %d", b+1, nb, ms, out.found, out.missed, out.hashBad, out.bytes)
		}
	}

	// Report the end.
	logf("done: found %d missed %d hashbad %d bytes %d", out.found, out.missed, out.hashBad, out.bytes)
	return 0, nil
}

// readKeys reads the unique keys of a list, keeping their order.
func readKeys(path string) ([][]byte, error) {
	// Open the list.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Parse each line's last field as a key and keep the first of each.
	var keys [][]byte
	seen := make(map[string]bool)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		key, err := hex.DecodeString(fields[len(fields)-1])
		if err != nil || len(key) != 36 || !bytes.HasPrefix(key, keyPrefix) {
			return nil, fmt.Errorf("keys line %d: not a 36-byte sha256 key", n)
		}
		if !seen[string(key)] {
			seen[string(key)] = true
			keys = append(keys, key)
		}
	}
	return keys, sc.Err()
}

// readEnv reads the database location from NAME=VALUE lines. Errors name the
// line number only, never its text.
func readEnv(path string) (*block_store_sql.Config, error) {
	// Read the file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("read the env file")
	}

	// Split it into names and values, naming a bad line by number only.
	vals := make(map[string]string)
	for i, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(k, "HYPERLINE_WORLD_SQL_") {
			return nil, fmt.Errorf("env file line %d is malformed", i+1)
		}
		vals[k] = v
	}

	// Build the location as hyperline's worldSQLConfig.location does.
	conf := &block_store_sql.Config{
		Host:          vals["HYPERLINE_WORLD_SQL_HOST"],
		Database:      vals["HYPERLINE_WORLD_SQL_DATABASE"],
		User:          vals["HYPERLINE_WORLD_SQL_USER"],
		Password:      vals["HYPERLINE_WORLD_SQL_PASSWORD"],
		Table:         vals["HYPERLINE_WORLD_SQL_TABLE_PREFIX"] + "blocks",
		TLS:           true,
		TLSCertSHA256: vals["HYPERLINE_WORLD_SQL_TLS_CERT_SHA256"],
	}

	// Require the login, then apply the port and TLS when set.
	if conf.Host == "" || conf.Database == "" || conf.User == "" || conf.Password == "" {
		return nil, errors.New("env file lacks the host, database, user or password")
	}
	if p := vals["HYPERLINE_WORLD_SQL_PORT"]; p != "" {
		port, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return nil, errors.New("env file port is not a port")
		}
		conf.Port = uint32(port)
	}
	if t := vals["HYPERLINE_WORLD_SQL_TLS"]; t != "" {
		tls, err := strconv.ParseBool(t)
		if err != nil {
			return nil, errors.New("env file TLS is not a boolean")
		}
		conf.TLS = tls
	}
	return conf, nil
}

// fatal reports an error that stops the run without a reconnect: a refused
// connection, too many connections, or a refused login.
func fatal(err error) bool {
	// Match the server's refusal numbers.
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1040, 1044, 1045, 1129, 1130, 1203, 1226, 1698:
			return true
		}
	}

	// Match a refused dial or the same words in other errors.
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "refused") || strings.Contains(s, "too many") || strings.Contains(s, "access denied")
}

// reader holds the one database session.
type reader struct {
	conf    *block_store_sql.Config
	table   string
	storeID []byte
	db      *sql.DB
	conn    *sql.Conn
}

func (r *reader) connect(ctx context.Context) error {
	// Open the pool, held to one connection that never recycles.
	db, err := block_store_sql.Open(r.conf)
	if err != nil {
		return errors.New("open the database config")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	// Take the session.
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := db.Conn(cctx)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("connect: %w", scrub(err))
	}
	r.db, r.conn = db, conn

	// Ask the server to make the session read only.
	if _, err := conn.ExecContext(cctx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
		logf("session read only: not accepted (%v)", scrub(err))
	} else {
		logf("session read only: accepted")
	}
	return nil
}

func (r *reader) close() {
	// Close the session, then the pool.
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
	if r.db != nil {
		_ = r.db.Close()
		r.db = nil
	}
}

// row is one found block.
type row struct {
	refs  []byte
	known bool
	data  []byte
}

// read runs the store's batch query for keys over the session.
func (r *reader) read(ctx context.Context, keys [][]byte) (map[string]row, int64, error) {
	// Build the store's batch query.
	args := make([]any, 0, len(keys)+1)
	args = append(args, r.storeID)
	for _, k := range keys {
		args = append(args, k)
	}
	query := "SELECT hash, refs, data FROM " + r.table + " WHERE store_id = ? AND hash IN (" + // #nosec G202 -- the table is quoted and values are placeholders.
		strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + ")"

	// Run it.
	qctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	start := time.Now()
	rows, err := r.conn.QueryContext(qctx, query, args...)
	if err != nil {
		return nil, 0, scrub(err)
	}
	defer rows.Close()

	// Collect the rows by key.
	got := make(map[string]row, len(keys))
	for rows.Next() {
		var key, refs, data []byte
		if err := rows.Scan(&key, &refs, &data); err != nil {
			return nil, 0, scrub(err)
		}
		got[string(key)] = row{refs: refs, known: refs != nil, data: data}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, scrub(err)
	}
	return got, time.Since(start).Milliseconds(), nil
}

// scrub keeps a driver error's number and class but drops any text that
// could hold the login: the driver's errors name no password, but an address
// error may name the user.
func scrub(err error) error {
	// Keep a server error's number and a refused dial's class.
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return &mysql.MySQLError{Number: me.Number, Message: "(message dropped)"}
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("connection refused: %w", syscall.ECONNREFUSED)
	}

	// Name a timeout, and cut anything before an address's "@".
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout")
	}
	s := err.Error()
	if i := strings.Index(s, "@"); i >= 0 {
		s = "(redacted)" + s[i:]
	}
	return errors.New(s)
}

// decodeRefs decodes the refs column: a sequence of uvarint-length-prefixed
// marshaled refs (spacewave db/block/store/sql encodeRefs).
func decodeRefs(data []byte) ([]*block.BlockRef, error) {
	// Read each length-prefixed ref.
	refs := []*block.BlockRef{}
	for len(data) != 0 {
		n, w := binary.Uvarint(data)
		if w <= 0 || n > uint64(len(data)-w) { //nolint:gosec // len(data)-w is never negative after a good Uvarint.
			return nil, errors.New("decode block refs: truncated")
		}
		data = data[w:]
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data[:n]); err != nil {
			return nil, fmt.Errorf("decode block refs: %w", err)
		}
		refs = append(refs, ref)
		data = data[n:]
	}
	return refs, nil
}

// output is the frames, misses and hash failures, with the ledger.
type output struct {
	blocks, misses, bad, ledger *os.File
	first, next                 int
	found, missed, hashBad      int
	bytes                       int64
}

// openOutput opens the output files and resumes from the ledger: the last
// complete line names the next batch and the sizes the files are cut back to.
func openOutput(dir, name string) (*output, error) {
	// Open the four files.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	o := &output{}
	open := func(ext string) (*os.File, error) {
		return os.OpenFile(filepath.Join(dir, name+ext), os.O_RDWR|os.O_CREATE, 0o600)
	}
	var err error
	if o.blocks, err = open(".blocks"); err != nil {
		return nil, err
	}
	if o.misses, err = open(".misses"); err != nil {
		return nil, err
	}
	if o.bad, err = open(".hashbad"); err != nil {
		return nil, err
	}
	if o.ledger, err = open(".ledger"); err != nil {
		return nil, err
	}

	// Read the last complete ledger line.
	data, err := os.ReadFile(o.ledger.Name())
	if err != nil {
		return nil, err
	}
	var sizes [4]int64
	var good int64
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		var b, f, m, h int
		var by, s0, s1, s2 int64
		if _, err := fmt.Sscanf(line, "batch %d found %d missed %d hashbad %d bytes %d sizes %d %d %d\n", &b, &f, &m, &h, &by, &s0, &s1, &s2); err != nil {
			return nil, fmt.Errorf("ledger line after offset %d is malformed", good)
		}
		o.next, o.found, o.missed, o.hashBad, o.bytes = b+1, f, m, h, by
		sizes = [4]int64{s0, s1, s2}
		good += int64(len(line))
	}
	sizes[3] = good

	// Cut each file back to the sizes that line records.
	for i, f := range []*os.File{o.blocks, o.misses, o.bad, o.ledger} {
		if err := f.Truncate(sizes[i]); err != nil {
			return nil, err
		}
		if _, err := f.Seek(sizes[i], 0); err != nil {
			return nil, err
		}
	}
	o.first = o.next
	return o, nil
}

// write appends one batch's frames and misses, syncs them, then records the
// batch in the ledger.
func (o *output) write(b int, keys [][]byte, rows map[string]row) error {
	// Frame each verified block; list the misses and hash failures.
	var frames, misses, bad bytes.Buffer
	for _, key := range keys {
		r, ok := rows[string(key)]
		if !ok {
			misses.WriteString(hex.EncodeToString(key) + "\n")
			o.missed++
			continue
		}
		sum := sha256.Sum256(r.data)
		if !bytes.Equal(sum[:], key[4:]) {
			bad.WriteString(hex.EncodeToString(key) + "\n")
			o.hashBad++
			continue
		}
		var refs []*block.BlockRef
		if r.known {
			var err error
			if refs, err = decodeRefs(r.refs); err != nil {
				bad.WriteString(hex.EncodeToString(key) + " refs\n")
				o.hashBad++
				continue
			}
		}
		obj, err := block.EncodeBlockObject(r.data, refs, r.known)
		if err != nil {
			return err
		}
		known := uint64(0)
		if r.known {
			known = 1
		}
		frames.Write(binary.AppendUvarint(nil, uint64(len(key))))
		frames.Write(key)
		frames.Write(binary.AppendUvarint(nil, known))
		frames.Write(binary.AppendUvarint(nil, uint64(len(obj))))
		frames.Write(obj)
		o.found++
		o.bytes += int64(len(r.data))
	}

	// Append and sync the frames, misses and failures.
	var sizes [3]int64
	for i, w := range []struct {
		f *os.File
		b *bytes.Buffer
	}{{o.blocks, &frames}, {o.misses, &misses}, {o.bad, &bad}} {
		if _, err := w.f.Write(w.b.Bytes()); err != nil {
			return err
		}
		if err := w.f.Sync(); err != nil {
			return err
		}
		off, err := w.f.Seek(0, 1)
		if err != nil {
			return err
		}
		sizes[i] = off
	}

	// Record the batch and the sizes after it in the ledger.
	line := fmt.Sprintf("batch %d found %d missed %d hashbad %d bytes %d sizes %d %d %d\n", b, o.found, o.missed, o.hashBad, o.bytes, sizes[0], sizes[1], sizes[2])
	if _, err := o.ledger.WriteString(line); err != nil {
		return err
	}
	return o.ledger.Sync()
}

func (o *output) close() {
	// Close each open file.
	for _, f := range []*os.File{o.blocks, o.misses, o.bad, o.ledger} {
		if f != nil {
			_ = f.Close()
		}
	}
}

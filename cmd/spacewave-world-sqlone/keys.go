//go:build !js && !wasip1 && !windows

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Bounds of the key listing: rows per query, queries per second, and the
// longest query before the listing stops.
const (
	listBatchMax = 10000
	listRateMax  = 2
	listQueryMax = 10 * time.Second
)

// pageFunc returns up to n keys of the store above after, in key order, and
// the query's duration.
type pageFunc func(ctx context.Context, after []byte, n int) ([][]byte, time.Duration, error)

// listKeys streams every key of the store into the output, a page at a time:
// at most batch keys per query and one query per interval. Any error, and any
// query that takes longer than maxQuery, stops the listing; the ledger keeps
// every page written before it, so a later run resumes after the last key.
func listKeys(ctx context.Context, page pageFunc, out *keyOutput, batch int, interval, maxQuery time.Duration, stopFile string) (int, error) {
	// Stop when the ledger already holds the last page.
	logf("list resume at page %d after %d keys", out.pages, out.keys)
	if out.end {
		logf("done: keys %d pages %d", out.keys, out.pages)
		return 0, nil
	}

	// Read the pages in order at the rate until a short page.
	last := time.Time{}
	lastLog := time.Now()
	for {
		// Stop cleanly on the stop file.
		if stopFile != "" {
			if _, err := os.Stat(stopFile); err == nil {
				logf("stop file present, stopping at page %d", out.pages)
				return 3, nil
			}
		}

		// Wait out the rate, then query the page.
		if wait := interval - time.Since(last); wait > 0 {
			time.Sleep(wait)
		}
		last = time.Now()
		keys, took, err := page(ctx, out.last, batch)
		if err != nil {
			return exitStop, fmt.Errorf("page %d: %w", out.pages, err)
		}

		// Record the page, then stop on a slow query.
		if err := out.write(keys, len(keys) < batch); err != nil {
			return 1, err
		}
		if took > maxQuery {
			return exitStop, fmt.Errorf("page %d took %d ms, over %d ms", out.pages-1, took.Milliseconds(), maxQuery.Milliseconds())
		}

		// Report progress, and end after the short page.
		if out.end || out.pages == 1 || time.Since(lastLog) >= 30*time.Second {
			lastLog = time.Now()
			logf("page %d query_ms %d keys %d", out.pages, took.Milliseconds(), out.keys)
		}
		if out.end {
			logf("done: keys %d pages %d", out.keys, out.pages)
			return 0, nil
		}
	}
}

// keyOutput is the key list, one hex key per line, with its ledger.
type keyOutput struct {
	// list is the key list file.
	list *os.File
	// ledger is the append-only record of the pages written.
	ledger *os.File
	// pages is the number of pages written.
	pages int
	// keys is the number of keys written.
	keys int64
	// last is the last key written, nil before the first.
	last []byte
	// end records that the last page written was short: the listing is done.
	end bool
}

// openKeyOutput opens the key list and its ledger and resumes from the last
// complete ledger line, cutting the list back to the size that line records.
func openKeyOutput(dir, name string) (*keyOutput, error) {
	// Open the two files.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	o := &keyOutput{}
	var err error
	if o.list, err = os.OpenFile(filepath.Join(dir, name+".keys"), os.O_RDWR|os.O_CREATE, 0o600); err != nil {
		return nil, err
	}
	if o.ledger, err = os.OpenFile(filepath.Join(dir, name+".keyledger"), os.O_RDWR|os.O_CREATE, 0o600); err != nil {
		o.close()
		return nil, err
	}

	// Read the last complete ledger line.
	size, good, err := o.readLedger()
	if err != nil {
		o.close()
		return nil, err
	}

	// Cut each file back to that line.
	for _, c := range []struct {
		f *os.File
		n int64
	}{{o.list, size}, {o.ledger, good}} {
		if err := c.f.Truncate(c.n); err != nil {
			o.close()
			return nil, err
		}
		if _, err := c.f.Seek(c.n, 0); err != nil {
			o.close()
			return nil, err
		}
	}
	return o, nil
}

// readLedger applies the ledger's complete lines and returns the list size
// the last one records and the ledger bytes they span.
func (o *keyOutput) readLedger() (int64, int64, error) {
	// Read the whole ledger: one short line per page.
	data, err := os.ReadFile(o.ledger.Name())
	if err != nil {
		return 0, 0, err
	}

	// Parse each complete line; a torn last line is dropped.
	var size, good int64
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		var page, end int
		var keys, sz int64
		var last string
		if _, err := fmt.Sscanf(line, "page %d keys %d last %s size %d end %d\n", &page, &keys, &last, &sz, &end); err != nil {
			return 0, 0, fmt.Errorf("key ledger line after offset %d is malformed", good)
		}

		// Take the line's position, decoding its last key.
		if o.last, err = decodeLast(last); err != nil {
			return 0, 0, fmt.Errorf("key ledger line after offset %d: %w", good, err)
		}
		o.pages, o.keys, o.end, size = page+1, keys, end == 1, sz
		good += int64(len(line))
	}
	return size, good, nil
}

// decodeLast decodes a ledger line's last key, "-" before the first key.
func decodeLast(s string) ([]byte, error) {
	if s == "-" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

// write appends one page's keys, syncs them, then records the page.
func (o *keyOutput) write(keys [][]byte, end bool) error {
	// Append and sync the keys.
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(hex.EncodeToString(k))
		b.WriteByte('\n')
	}
	if _, err := o.list.WriteString(b.String()); err != nil {
		return err
	}
	if err := o.list.Sync(); err != nil {
		return err
	}

	// Advance the position past the page.
	size, err := o.list.Seek(0, 1)
	if err != nil {
		return err
	}
	if len(keys) != 0 {
		o.last = keys[len(keys)-1]
	}
	o.keys += int64(len(keys))
	o.end = end

	// Record the page in the ledger.
	last, flag := "-", 0
	if o.last != nil {
		last = hex.EncodeToString(o.last)
	}
	if end {
		flag = 1
	}
	if _, err := fmt.Fprintf(o.ledger, "page %d keys %d last %s size %d end %d\n", o.pages, o.keys, last, size, flag); err != nil {
		return err
	}
	o.pages++
	return o.ledger.Sync()
}

func (o *keyOutput) close() {
	// Close each open file.
	for _, f := range []*os.File{o.list, o.ledger} {
		if f != nil {
			_ = f.Close()
		}
	}
}

// page reads up to n keys of the store above after over the session, in key
// order, bounded by the listing's longest query.
func (r *reader) page(ctx context.Context, after []byte, n int) ([][]byte, time.Duration, error) {
	// Query the next keys after the last one, on the primary key.
	qctx, cancel := context.WithTimeout(ctx, listQueryMax+5*time.Second)
	defer cancel()
	if after == nil {
		after = []byte{}
	}
	start := time.Now()
	rows, err := r.conn.QueryContext(qctx, "SELECT hash FROM "+r.table+" WHERE store_id = ? AND hash > ? ORDER BY hash LIMIT ?", r.storeID, after, n) // #nosec G202 -- the table is quoted and values are placeholders.
	if err != nil {
		return nil, 0, scrub(err)
	}
	defer rows.Close()

	// Collect the keys.
	keys := make([][]byte, 0, n)
	for rows.Next() {
		var key []byte
		if err := rows.Scan(&key); err != nil {
			return nil, 0, scrub(err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, scrub(err)
	}
	return keys, time.Since(start), nil
}

// estimate logs the server's estimate of the table's rows and bytes, read
// from information_schema: metadata only, no table scan.
func (r *reader) estimate(ctx context.Context) error {
	// Read the table's statistics row.
	qctx, cancel := context.WithTimeout(ctx, listQueryMax)
	defer cancel()
	var rows, data, index int64
	err := r.conn.QueryRowContext(qctx, "SELECT COALESCE(TABLE_ROWS, -1), COALESCE(DATA_LENGTH, -1), COALESCE(INDEX_LENGTH, -1) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", r.conf.Table).Scan(&rows, &data, &index)
	if err != nil {
		return scrub(err)
	}
	logf("estimate: table rows %d data bytes %d index bytes %d (all stores)", rows, data, index)
	return nil
}

// errNotReadOnly is the listing's refusal of a session the server did not
// make read only.
var errNotReadOnly = errors.New("the server did not accept a read-only session")

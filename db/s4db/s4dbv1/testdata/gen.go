//go:build ignore

// gen writes the s4dbv1 test fixtures with the engine that wrote the format
// this package reads. Run it from a checkout of that engine, the master
// branch before the rewrite, naming this directory:
//
//	go run db/s4db/s4dbv1/testdata/gen.go db/s4db/s4dbv1/testdata
//
// It writes mixed.s4db, a file whose keys sit in the checkpointed tree and in
// the log after it, with values inline and in extents, overwrites, deletes
// and a log that crosses a chunk link; mixed.expected.tsv, every key the file
// holds with the length and SHA-256 of its value; mixed.before-last.tsv, the
// same before the file's last commit, which a torn-tail test cuts; and
// empty.s4db, a file no commit ever reached.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/s4wave/spacewave/db/s4db"
)

// expected is the state the fixture should stream: key to value.
type expected map[string][]byte

// writeTSV writes e to path as key, length, SHA-256 rows in key order.
func writeTSV(path string, e expected) error {
	// Order the keys.
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Write one row per key.
	var sb strings.Builder
	for _, k := range keys {
		sum := sha256.Sum256(e[k])
		fmt.Fprintf(&sb, "%s\t%d\t%s\n", k, len(e[k]), hex.EncodeToString(sum[:]))
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// val returns a value of n bytes derived from key and a generation, so a
// rewrite of the key changes every byte.
func val(key string, gen, n int) []byte {
	pat := fmt.Sprintf("%s/%d|", key, gen)
	b := make([]byte, 0, n+len(pat))
	for len(b) < n {
		b = append(b, pat...)
	}
	return b[:n]
}

// batch applies sets and deletes in one commit and mirrors them in e.
func batch(ctx context.Context, db *s4db.DB, e expected, sets map[string][]byte, dels []string) error {
	// Open the transaction.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Apply the sets, then the deletes.
	for k, v := range sets {
		if err := tx.Set(ctx, []byte(k), v); err != nil {
			return err
		}
		e[k] = v
	}
	for _, k := range dels {
		if err := tx.Delete(ctx, []byte(k)); err != nil {
			return err
		}
		delete(e, k)
	}
	return tx.Commit(ctx)
}

// size returns the value size of key number i in generation gen: every
// fifth key takes an extent, the rest stay inline, and a later generation
// swaps the two so overwrites cross the inline bound both ways.
func size(i, gen int) int {
	// Pick the kind, then its size.
	ext := i%5 == 0
	if gen%2 == 1 {
		ext = !ext
	}
	if ext {
		return 2000 + i%700
	}
	return 20 + i%300
}

// run writes the fixtures into dir.
func run(dir string) error {
	// An empty file: created, never written.
	ctx := context.Background()
	empty, err := s4db.Open(filepath.Join(dir, "empty.s4db"), s4db.Options{RelocateBudget: -1})
	if err != nil {
		return err
	}
	if err := empty.Close(); err != nil {
		return err
	}

	// The mixed file.
	path := filepath.Join(dir, "mixed.s4db")
	_ = os.Remove(path)
	db, err := s4db.Open(path, s4db.Options{RelocateBudget: -1})
	if err != nil {
		return err
	}
	e := expected{}

	// Keys for the tree: t keys to keep, d keys to delete later, and x keys
	// deleted before the checkpoint. First the sets.
	sets := map[string][]byte{}
	for i := range 1500 {
		k := fmt.Sprintf("t%05d", i)
		sets[k] = val(k, 0, size(i, 0))
	}
	for i := range 200 {
		k := fmt.Sprintf("d%05d", i)
		sets[k] = val(k, 0, size(i, 0))
	}
	for i := range 50 {
		k := fmt.Sprintf("x%05d", i)
		sets[k] = val(k, 0, size(i, 0))
	}
	if err := batch(ctx, db, e, sets, nil); err != nil {
		return err
	}

	// Then the x deletes.
	var dels []string
	for i := range 50 {
		dels = append(dels, fmt.Sprintf("x%05d", i))
	}
	if err := batch(ctx, db, e, nil, dels); err != nil {
		return err
	}

	// Checkpoint: everything so far moves into the tree.
	if err := db.Compact(ctx); err != nil {
		return err
	}

	// The log after the checkpoint: overwrites crossing the inline bound,
	// deletes of tree keys, new keys, a key set and deleted, a key set,
	// deleted and set again. First the overwrites.
	sets = map[string][]byte{}
	for i := 0; i < 1500; i += 3 {
		k := fmt.Sprintf("t%05d", i)
		sets[k] = val(k, 1, size(i, 1))
	}
	if err := batch(ctx, db, e, sets, nil); err != nil {
		return err
	}

	// The deletes.
	dels = nil
	for i := range 200 {
		dels = append(dels, fmt.Sprintf("d%05d", i))
	}
	for i := 0; i < 1500; i += 17 {
		dels = append(dels, fmt.Sprintf("t%05d", i))
	}
	if err := batch(ctx, db, e, nil, dels); err != nil {
		return err
	}

	// The new keys and the r keys' first values.
	sets = map[string][]byte{}
	for i := range 300 {
		k := fmt.Sprintf("n%05d", i)
		sets[k] = val(k, 0, size(i, 0))
	}
	sets["r00001"] = val("r00001", 0, 3000)
	sets["r00002"] = val("r00002", 0, 100)
	if err := batch(ctx, db, e, sets, nil); err != nil {
		return err
	}

	// The r keys deleted, then one set again.
	if err := batch(ctx, db, e, nil, []string{"r00001", "r00002"}); err != nil {
		return err
	}
	if err := batch(ctx, db, e, map[string][]byte{"r00002": val("r00002", 1, 2500)}, nil); err != nil {
		return err
	}

	// Enough inline values to carry the log past its first chunk, so the
	// reader follows a link record.
	for c := range 10 {
		sets = map[string][]byte{}
		for i := c * 260; i < (c+1)*260; i++ {
			k := fmt.Sprintf("l%05d", i)
			sets[k] = val(k, 0, 400+i%100)
		}
		if err := batch(ctx, db, e, sets, nil); err != nil {
			return err
		}
	}

	// Record the state before the last commit.
	if err := writeTSV(filepath.Join(dir, "mixed.before-last.tsv"), e); err != nil {
		return err
	}

	// The last commit: new keys of both kinds, an overwrite and a delete.
	sets = map[string][]byte{}
	for i := range 10 {
		k := fmt.Sprintf("z%05d", i)
		sets[k] = val(k, 0, size(i, 0))
	}
	sets["t00001"] = val("t00001", 2, 4000)
	if err := batch(ctx, db, e, sets, []string{"n00000"}); err != nil {
		return err
	}

	// Close and record the final state.
	if err := db.Close(); err != nil {
		return err
	}
	return writeTSV(filepath.Join(dir, "mixed.expected.tsv"), e)
}

func main() {
	// Take the output directory and run.
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen <testdata dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

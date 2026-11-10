package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// testBlock is block i of buildTestStore: its hash, b58 form, block key,
// and h row key in the pack index, without the prefix.
type testBlock struct {
	h        *hash.Hash
	b58      string
	key      []byte
	readKey  []byte
	hashRow  []byte
	category int
}

func testBlockOf(i int) testBlock {
	d := sha256.Sum256([]byte("block-" + strconv.Itoa(i)))
	h := hash.NewHash(hash.HashType_HashType_SHA256, d[:])
	rk, _ := block.NewBlockRef(h).MarshalKey()
	md := h.MarshalDigest()
	return testBlock{
		h:        h,
		b58:      h.MarshalString(),
		key:      append([]byte(blockPrefix), rk...),
		readKey:  d[:readKeyLen],
		hashRow:  append([]byte("h"), md[len(md)-indexKeyBytes:]...),
		category: i % 10,
	}
}

// dropBlock is a block buildTestStore's rule drops: queued at epoch 2,
// unmarked, packed, cold.
var dropBlock = testBlockOf(5)

// kvRow is a row a test adds to the World or the pack index.
type kvRow struct {
	key   []byte
	value any
}

// ruleWorld builds the test store with the World rows world and the pack
// index rows packs (keys after the pack index prefix) added; a row with a
// nil value deletes the key.
func ruleWorld(t *testing.T, world, packs []kvRow) opts {
	t.Helper()
	o := sharedRefWorld(t, func(_, _ string) []string { return nil })
	put := func(path string, pre []byte, rows []kvRow) {
		db := mustOpen(t, path)
		for _, r := range rows {
			k := append(append([]byte{}, pre...), r.key...)
			var err error
			if r.value == nil {
				_, err = db.Exec("DELETE FROM kv WHERE key = ?", k)
			} else {
				_, err = db.Exec("INSERT OR REPLACE INTO kv (key, value) VALUES (?, ?)", k, r.value)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	put(o.world, nil, world)
	sum := sha256.Sum256([]byte(o.storeID))
	put(o.packs, append([]byte("pi"), sum[:8]...), packs)
	return o
}

// own returns a key of the rebuilt write-back store.
func own(rest string) []byte { return []byte(testObjPrefix + rest) }

// copyHolds runs copy with read and reports whether the copy holds key.
func copyHolds(t *testing.T, o opts, read string, key []byte) (bool, error) {
	t.Helper()
	o.read = read
	o.out = filepath.Join(t.TempDir(), "new.db")
	if err := runCopy(o); err != nil {
		return false, err
	}
	var n int
	if err := mustOpen(t, o.out).QueryRow("SELECT count(*) FROM kv WHERE key = ?", key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1, nil
}

// mustRefuse checks count, copy -read raw and copy -read sqlite each refuse
// the World, or at the least keep the drop block.
func mustRefuse(t *testing.T, o opts, what string) {
	t.Helper()
	if _, err := runCount(o); err == nil {
		t.Errorf("count succeeded with %s", what)
	}
	for _, read := range []string{"raw", "sqlite"} {
		held, err := copyHolds(t, o, read, dropBlock.key)
		if err == nil {
			t.Errorf("copy -read %s succeeded with %s (drop block kept: %v)", read, what, held)
		}
	}
}

// TestRuleDropBlockDrops checks the drop block of the tests below is dropped
// on the unchanged store, so each refusal is the row's doing.
func TestRuleDropBlockDrops(t *testing.T) {
	o := ruleWorld(t, nil, nil)
	for _, read := range []string{"raw", "sqlite"} {
		held, err := copyHolds(t, o, read, dropBlock.key)
		if err != nil {
			t.Fatal(err)
		}
		if held {
			t.Errorf("copy -read %s kept the drop block", read)
		}
	}
}

// TestRuleRefusesMarked checks a marked/ row of the rebuilt store whose key
// is no block hash, or whose cell does not decode as a blob, refuses the run.
func TestRuleRefusesMarked(t *testing.T) {
	for name, row := range map[string]kvRow{
		"key not a hash":    {own("marked/0OIl0OIl"), []byte{}},
		"empty hash":        {own("marked/"), []byte{}},
		"integer value":     {own("marked/" + dropBlock.b58), 5},
		"key with a suffix": {own("marked/" + dropBlock.b58 + "/x"), []byte{}},
	} {
		t.Run(name, func(t *testing.T) {
			o := ruleWorld(t, []kvRow{row}, nil)
			if name == "integer value" {
				// The marker is a marker: SQLite reads keep the block;
				// count's raw pass cannot decode the cell and must refuse.
				if _, err := runCount(o); err == nil {
					t.Error("count succeeded with a marked/ cell it cannot decode")
				}
				for _, read := range []string{"raw", "sqlite"} {
					if held, err := copyHolds(t, o, read, dropBlock.key); err == nil && !held {
						t.Errorf("copy -read %s dropped a marked block", read)
					}
				}
				return
			}
			mustRefuse(t, o, "a marked/ row "+name)
		})
	}
}

// TestRuleRefusesEvictable checks an evictable/ row or epoch the store's own
// parsers refuse refuses the run.
func TestRuleRefusesEvictable(t *testing.T) {
	for name, row := range map[string]kvRow{
		"epoch not hex":   {own("evictable/zzzzzzzzzzzzzzzz/" + dropBlock.b58), []byte("10")},
		"short epoch":     {own("evictable/01/" + dropBlock.b58), []byte("10")},
		"hash not b58":    {own("evictable/0000000000000001/0OIl0OIl"), []byte("10")},
		"no hash":         {own("evictable/0000000000000001"), []byte("10")},
		"epoch value bad": {own("epoch"), []byte("x")},
		"integer cell":    {own("evictable/0000000000000001/" + testBlockOf(15).b58), 5},
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, ruleWorld(t, []kvRow{row}, nil), "an evictable/ row "+name)
		})
	}
}

// TestRuleEpochBoundsWalk checks an entry queued at or after the epoch the
// next pass advances to is not walked, so its block is kept.
func TestRuleEpochBoundsWalk(t *testing.T) {
	// Epoch 1: the next pass is epoch 2 and walks the entries of epochs 0
	// and 1; block 5 is queued at epoch 2, block 6 at epoch 0.
	o := ruleWorld(t, []kvRow{{own("epoch"), []byte("1")}}, nil)
	for _, read := range []string{"raw", "sqlite"} {
		held, err := copyHolds(t, o, read, dropBlock.key)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			t.Errorf("copy -read %s dropped a block queued at the next pass's epoch", read)
		}
		if held, _ := copyHolds(t, o, read, testBlockOf(6).key); held {
			t.Errorf("copy -read %s kept a block the next pass drops", read)
		}
	}
}

// TestRuleRefusesReads checks a reads/ row parseReadsStamp refuses, above
// MaxInt64 or not hex, or a chunk cell that does not decode, refuses the run.
func TestRuleRefusesReads(t *testing.T) {
	for name, row := range map[string]kvRow{
		"MaxUint64":           {own("reads/ffffffffffffffff/0000000000000001"), []byte("zzzzzzzzzzzzzzzz")},
		"above MaxInt64":      {own("reads/fffffffffffffffe/0000000000000001"), []byte("zzzzzzzzzzzzzzzz")},
		"MaxInt64 + 1":        {own("reads/8000000000000000/0000000000000001"), []byte("zzzzzzzzzzzzzzzz")},
		"uppercase above":     {own("reads/FFFFFFFFFFFFFFFF/0000000000000001"), []byte("zzzzzzzzzzzzzzzz")},
		"not hex":             {own("reads/zzzzzzzzzzzzzzzz/0000000000000001"), []byte("zzzzzzzzzzzzzzzz")},
		"short":               {own("reads/0001"), []byte("zzzzzzzzzzzzzzzz")},
		"integer chunk value": {own("reads/7ffffffffffffff0/7ffffffffffffff1"), 5},
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, ruleWorld(t, []kvRow{row}, nil), "a reads/ row "+name)
		})
	}
}

// TestRuleReadsByValue checks the read generations are ordered by their
// decoded time, as LoadReads sorts them, uppercase hex included: the newest
// two by value are kept, not the newest two by string.
func TestRuleReadsByValue(t *testing.T) {
	other := []byte("0123456789abcdef")
	o := ruleWorld(t, []kvRow{
		// By value D00 > c00 > b00, so D00 and c00 are kept; by string
		// D00 < b00 < c00.
		{own("reads/7ffffffffffffc00/7ffffffffffffc01"), other},
		{own("reads/7ffffffffffffD00/7ffffffffffffd01"), dropBlock.readKey},
		{own("reads/7ffffffffffffb00/7ffffffffffffb01"), other},
	}, nil)
	for _, read := range []string{"raw", "sqlite"} {
		held, err := copyHolds(t, o, read, dropBlock.key)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			t.Errorf("copy -read %s dropped a block read in the newest generation", read)
		}
	}
	res, err := runCount(o)
	if err != nil {
		t.Fatal(err)
	}
	if v := res.blocks[vHot]; v == nil || v.keys != 1 {
		t.Errorf("count: hot %+v, want 1 (the old generation's hot blocks are aged out)", v)
	}
}

// TestRuleRefusesQueue checks a queue/ row the store's upload queue parser
// refuses refuses the run.
func TestRuleRefusesQueue(t *testing.T) {
	for name, row := range map[string]kvRow{
		"time not hex":   {own("queue/zzzzzzzzzzzzzzzz/" + dropBlock.b58), []byte("10")},
		"above MaxInt64": {own("queue/ffffffffffffffff/" + dropBlock.b58), []byte("10")},
		"hash not b58":   {own("queue/0000000000000001/0OIl0OIl"), []byte("10")},
		"no hash":        {own("queue/0000000000000001"), []byte("10")},
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, ruleWorld(t, []kvRow{row}, nil), "a queue/ row "+name)
		})
	}
}

// TestRuleQueuedKept checks a block in the upload queue is kept.
func TestRuleQueuedKept(t *testing.T) {
	o := ruleWorld(t, []kvRow{{own("queue/0000000000000001/" + dropBlock.b58), []byte("10")}}, nil)
	for _, read := range []string{"raw", "sqlite"} {
		held, err := copyHolds(t, o, read, dropBlock.key)
		if err != nil {
			t.Fatal(err)
		}
		if !held {
			t.Errorf("copy -read %s dropped a block awaiting upload", read)
		}
	}
}

// TestRuleRefusesPackRows checks a p/ row the pack table load refuses, or an
// h row of a drop candidate that does not decode, refuses the run.
func TestRuleRefusesPackRows(t *testing.T) {
	num := func(x uint32) []byte { return binary.BigEndian.AppendUint32(nil, x) }
	for name, row := range map[string]kvRow{
		"p/ row of 4 bytes": {[]byte("p/packC"), num(3)},
		"p/ row of 6 bytes": {[]byte("p/packC"), append(num(3), 1, 1)},
		"h row of 2 bytes":  {dropBlock.hashRow, []byte{0, 1}},
		"h row integer":     {dropBlock.hashRow, 1},
	} {
		t.Run(name, func(t *testing.T) {
			mustRefuse(t, ruleWorld(t, nil, []kvRow{row}), name)
		})
	}
}

// TestRuleRefusesUnknown checks a key of the rebuilt store that is none of
// its sub-keys refuses the run.
func TestRuleRefusesUnknown(t *testing.T) {
	for _, rest := range []string{"zzz", "marked", "evictable", "status/writeback/epoch", "reads", "queue"} {
		t.Run(rest, func(t *testing.T) {
			mustRefuse(t, ruleWorld(t, []kvRow{{own(rest), []byte("1")}}, nil), "an unknown key "+rest)
		})
	}
}

// TestNeededOutUniqueSynced checks count writes -needed-out through a temp
// file of its own in the same directory, synced before the rename, and
// leaves a file at the old fixed temp name alone.
func TestNeededOutUniqueSynced(t *testing.T) {
	o := ruleWorld(t, nil, nil)
	dir := t.TempDir()
	o.neededOut = filepath.Join(dir, "needed.txt")
	fixed := o.neededOut + ".tmp"
	if err := os.WriteFile(fixed, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	var synced []string
	saved := syncFile
	syncFile = func(f *os.File) error {
		if _, err := os.Stat(o.neededOut); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("needed-out in place before its temp file synced: %v", err)
		}
		synced = append(synced, f.Name())
		return saved(f)
	}
	defer func() { syncFile = saved }()
	if _, err := runCount(o); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(fixed); err != nil || string(b) != "other" {
		t.Errorf("%s: %q %v, want it untouched", fixed, b, err)
	}
	if len(synced) != 1 || filepath.Dir(synced[0]) != dir || synced[0] == fixed {
		t.Errorf("synced %v, want one temp file in %s other than %s", synced, dir, fixed)
	}
	if _, err := os.Stat(o.neededOut); err != nil {
		t.Error(err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 2 {
		t.Errorf("dir holds %d files, want needed-out and the old temp", len(ents))
	}
}

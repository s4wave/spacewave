//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
)

var iteratorTestBucket = []byte("iterator-test")

func TestIteratorPrefixBounds(t *testing.T) {
	keys := [][]byte{
		[]byte("aaa/1"),
		[]byte("aaa/2"),
		[]byte("bbb/other"),
		[]byte("mmm/1"),
		[]byte("mmm/2"),
		[]byte("yyy/other"),
		[]byte("zzz/1"),
		[]byte("zzz/2"),
	}
	db := openIteratorTestDB(t, keys)

	cases := []struct {
		name   string
		prefix []byte
		want   []string
	}{
		{name: "prefix at bucket start", prefix: []byte("aaa/"), want: []string{"aaa/1", "aaa/2"}},
		{name: "prefix in bucket middle", prefix: []byte("mmm/"), want: []string{"mmm/1", "mmm/2"}},
		{name: "prefix at bucket end", prefix: []byte("zzz/"), want: []string{"zzz/1", "zzz/2"}},
		{name: "empty prefix", want: []string{"aaa/1", "aaa/2", "bbb/other", "mmm/1", "mmm/2", "yyy/other", "zzz/1", "zzz/2"}},
	}

	for _, tc := range cases {
		for _, reverse := range []bool{false, true} {
			for _, startWithNext := range []bool{false, true} {
				direction := "forward"
				want := tc.want
				if reverse {
					direction = "reverse"
					want = reverseStrings(tc.want)
				}
				start := "Seek(nil)"
				if startWithNext {
					start = "Next"
				}
				t.Run(tc.name+"/"+direction+"/"+start, func(t *testing.T) {
					got := collectIteratorKeys(t, db, tc.prefix, reverse, startWithNext)
					if strings.Join(got, ",") != strings.Join(want, ",") {
						t.Fatalf("prefix iteration returned %q, want %q", got, want)
					}
				})
			}
		}
	}

	t.Run("empty bucket", func(t *testing.T) {
		emptyDB := openIteratorTestDB(t, nil)
		for _, reverse := range []bool{false, true} {
			if got := collectIteratorKeys(t, emptyDB, []byte("mmm/"), reverse, false); len(got) != 0 {
				t.Fatalf("empty bucket prefix iteration returned %q", got)
			}
			if got := collectIteratorKeys(t, emptyDB, nil, reverse, true); len(got) != 0 {
				t.Fatalf("empty bucket unprefixed iteration returned %q", got)
			}
		}
	})

	t.Run("seek before prefix", func(t *testing.T) {
		err := db.View(func(tx *bdb.Tx) error {
			bucket := tx.Bucket(iteratorTestBucket)
			forward := NewIterator(bucket.Cursor(), []byte("mmm/"), true, false)
			if err := forward.Seek([]byte("aaa/9")); err != nil {
				return err
			}
			if !forward.Valid() || string(forward.Key()) != "mmm/1" {
				t.Fatalf("forward seek before prefix landed on %q, want %q", forward.Key(), "mmm/1")
			}

			reverse := NewIterator(bucket.Cursor(), []byte("mmm/"), true, true)
			if err := reverse.Seek([]byte("aaa/9")); err != nil {
				return err
			}
			if reverse.Valid() {
				t.Fatalf("reverse seek before prefix landed on %q, want invalid", reverse.Key())
			}

			reverse = NewIterator(bucket.Cursor(), []byte("mmm/"), true, true)
			if err := reverse.Seek([]byte("yyy/other")); err != nil {
				return err
			}
			if !reverse.Valid() || string(reverse.Key()) != "mmm/2" {
				t.Fatalf("reverse seek after prefix landed on %q, want %q", reverse.Key(), "mmm/2")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reverse prefix without successor", func(t *testing.T) {
		binaryDB := openIteratorTestDB(t, [][]byte{{0xfe}, {0xff, 0x00}, {0xff, 0x01}})
		err := binaryDB.View(func(tx *bdb.Tx) error {
			iterator := NewIterator(tx.Bucket(iteratorTestBucket).Cursor(), []byte{0xff}, true, true)
			if err := iterator.Seek(nil); err != nil {
				return err
			}
			var got [][]byte
			for ; iterator.Valid(); iterator.Next() {
				got = append(got, bytes.Clone(iterator.Key()))
			}
			want := [][]byte{{0xff, 0x01}, {0xff, 0x00}}
			if len(got) != len(want) || !bytes.Equal(got[0], want[0]) || !bytes.Equal(got[1], want[1]) {
				t.Fatalf("reverse 0xff prefix returned %x, want %x", got, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestIteratorPrefixSeekCost(t *testing.T) {
	const iterations = 64

	for _, reverse := range []bool{false, true} {
		direction := "forward"
		if reverse {
			direction = "reverse"
		}
		t.Run(direction, func(t *testing.T) {
			small := measurePrefixSeek(t, 100, iterations, reverse)
			large := measurePrefixSeek(t, 100_000, iterations, reverse)
			t.Logf("100 filler: %s total for %d seeks", small, iterations)
			t.Logf("100000 filler: %s total for %d seeks", large, iterations)

			if large > small*20 {
				t.Fatalf("prefix seek cost grew more than 20x with unrelated filler: 100 keys=%s, 100000 keys=%s", small, large)
			}
		})
	}
}

func TestBoltScanPrefixSeekCost(t *testing.T) {
	const iterations = 64
	measure := func(filler int) time.Duration {
		db := openPrefixSeekDB(t, filler, false)
		rawTx, err := db.Begin(false)
		if err != nil {
			t.Fatal(err)
		}
		defer rawTx.Rollback()
		tx := NewTx(rawTx, iteratorTestBucket)
		scan := func() {
			count := 0
			err := tx.ScanPrefix(context.Background(), []byte("zzz/"), func(_, _ []byte) error {
				count++
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("ScanPrefix matched %d keys, want 1", count)
			}
		}
		for range 4 {
			scan()
		}
		started := time.Now()
		for range iterations {
			scan()
		}
		return time.Since(started)
	}

	small := measure(100)
	large := measure(100_000)
	t.Logf("100 filler: %s total for %d scans", small, iterations)
	t.Logf("100000 filler: %s total for %d scans", large, iterations)
	if large > small*20 {
		t.Fatalf("prefix scan cost grew more than 20x with unrelated filler: 100 keys=%s, 100000 keys=%s", small, large)
	}
}

func BenchmarkIteratorPrefixSeek(b *testing.B) {
	for _, filler := range []int{1_000, 10_000, 50_000, 100_000} {
		b.Run("filler="+strconv.Itoa(filler), func(b *testing.B) {
			db := openPrefixSeekDB(b, filler, false)
			tx, err := db.Begin(false)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = tx.Rollback() })
			bucket := tx.Bucket(iteratorTestBucket)

			b.ResetTimer()
			for range b.N {
				iterator := NewIterator(bucket.Cursor(), []byte("zzz/"), true, false)
				if err := iterator.Seek(nil); err != nil {
					b.Fatal(err)
				}
				if !iterator.Valid() || string(iterator.Key()) != "zzz/only" {
					b.Fatalf("prefix seek landed on %q", iterator.Key())
				}
			}
		})
	}
}

func collectIteratorKeys(t *testing.T, db *bdb.DB, prefix []byte, reverse, startWithNext bool) []string {
	t.Helper()
	var got []string
	err := db.View(func(tx *bdb.Tx) error {
		iterator := NewIterator(tx.Bucket(iteratorTestBucket).Cursor(), prefix, true, reverse)
		if startWithNext {
			iterator.Next()
		} else if err := iterator.Seek(nil); err != nil {
			return err
		}
		for ; iterator.Valid(); iterator.Next() {
			got = append(got, string(iterator.Key()))
		}
		return iterator.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func reverseStrings(values []string) []string {
	reversed := make([]string, len(values))
	for idx := range values {
		reversed[len(values)-idx-1] = values[idx]
	}
	return reversed
}

func measurePrefixSeek(t *testing.T, filler, iterations int, reverse bool) time.Duration {
	t.Helper()
	db := openPrefixSeekDB(t, filler, reverse)
	tx, err := db.Begin(false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	bucket := tx.Bucket(iteratorTestBucket)

	targetPrefix, targetKey := prefixSeekTarget(reverse)
	for range 4 {
		iterator := NewIterator(bucket.Cursor(), targetPrefix, true, reverse)
		if err := iterator.Seek(nil); err != nil {
			t.Fatal(err)
		}
	}

	started := time.Now()
	for range iterations {
		iterator := NewIterator(bucket.Cursor(), targetPrefix, true, reverse)
		if err := iterator.Seek(nil); err != nil {
			t.Fatal(err)
		}
		if !iterator.Valid() || !bytes.Equal(iterator.Key(), targetKey) {
			t.Fatalf("prefix seek landed on %q", iterator.Key())
		}
	}
	return time.Since(started)
}

func openPrefixSeekDB(tb testing.TB, filler int, reverse bool) *bdb.DB {
	tb.Helper()
	fillerPrefix := []byte("aaa/")
	if reverse {
		fillerPrefix = []byte("zzz/")
	}
	keys := make([][]byte, 0, filler+1)
	for idx := range filler {
		key := append(bytes.Clone(fillerPrefix), strconv.AppendInt(nil, int64(idx), 10)...)
		keys = append(keys, key)
	}
	_, targetKey := prefixSeekTarget(reverse)
	keys = append(keys, targetKey)
	return openIteratorTestDB(tb, keys)
}

func prefixSeekTarget(reverse bool) ([]byte, []byte) {
	if reverse {
		return []byte("aaa/"), []byte("aaa/only")
	}
	return []byte("zzz/"), []byte("zzz/only")
}

func openIteratorTestDB(tb testing.TB, keys [][]byte) *bdb.DB {
	tb.Helper()
	db, err := bdb.Open(filepath.Join(tb.TempDir(), "iterator.db"), 0o600, nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close iterator test database: %v", err)
		}
	})
	if err := db.Update(func(tx *bdb.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(iteratorTestBucket)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := bucket.Put(key, []byte{1}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	return db
}

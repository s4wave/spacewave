package kvtx_kvfile

import (
	"bytes"
	"io"
	"testing"

	"github.com/paralin/go-kvfile"
)

func TestKvfile(t *testing.T) {
	var buf bytes.Buffer
	keys := [][]byte{
		[]byte("test-1"),
		[]byte("test-2"),
		[]byte("test-3"),
	}
	vals := [][]byte{
		[]byte("val-1"),
		[]byte("val-2"),
		[]byte("val-3"),
	}
	// we write the keys in sequential order, use that here:
	var index int
	err := kvfile.Write(&buf, keys, func(wr io.Writer, key []byte) (uint64, error) {
		nw, err := wr.Write(vals[index])
		if err != nil {
			return 0, err
		}
		index++
		return uint64(nw), nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	bufReader := bytes.NewReader(buf.Bytes())
	rdr, err := kvfile.BuildReader(bufReader, uint64(buf.Len()))
	if err != nil {
		t.Fatal(err.Error())
	}

	store := NewKvfileStore(rdr)
	tx, err := store.NewTransaction(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	it := tx.Iterate([]byte("test-"), true, false)
	var n int
	for it.Next() {
		n++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err.Error())
	}
	if n != 3 {
		t.FailNow()
	}

	it = tx.Iterate([]byte("test-2"), true, false)
	n = 0
	for it.Next() {
		n++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err.Error())
	}
	if n != 1 {
		t.FailNow()
	}

	dat, found, err := tx.Get([]byte("test-2"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected to find test-2 key")
	}
	if !bytes.Equal(dat, vals[1]) {
		t.Fail()
	}

	n = 0
	err = tx.ScanPrefix([]byte("test"), func(key, value []byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != 3 {
		t.Fail()
	}

	tx.Discard()
}

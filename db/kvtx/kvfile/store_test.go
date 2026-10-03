package kvtx_kvfile

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
)

func TestKvfile(t *testing.T) {
	// Prepare ordered kvfile keys and their corresponding values.
	ctx := context.Background()
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

	// Write the ordered values into the kvfile buffer.
	// we write the keys in sequential order, use that here:
	var index int
	err := kvfile.Write(&buf, keys, func(wr io.Writer, key []byte) (uint64, error) {
		nw, err := wr.Write(vals[index])
		if err != nil {
			return 0, err
		}
		index++
		return uint64(nw), nil //nolint:gosec
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a kvfile reader over the completed buffer.
	bufReader := bytes.NewReader(buf.Bytes())
	rdr, err := kvfile.BuildReader(bufReader, uint64(buf.Len())) //nolint:gosec
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction on the kvfile store.
	store := NewKvfileStore(rdr)
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Count all entries matching the shared key prefix.
	it := tx.Iterate(ctx, []byte("test-"), true, false)
	var n int
	for it.Next() {
		n++
	}

	// Verify the shared prefix returns all three entries without error.
	if err := it.Err(); err != nil {
		t.Fatal(err.Error())
	}
	if n != 3 {
		t.FailNow()
	}

	// Count entries matching the exact second-key prefix.
	it = tx.Iterate(ctx, []byte("test-2"), true, false)
	n = 0
	for it.Next() {
		n++
	}

	// Verify the second-key prefix returns one entry without error.
	if err := it.Err(); err != nil {
		t.Fatal(err.Error())
	}
	if n != 1 {
		t.FailNow()
	}

	// Read the second key and verify its stored value.
	dat, found, err := tx.Get(ctx, []byte("test-2"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected to find test-2 key")
	}
	if !bytes.Equal(dat, vals[1]) {
		t.Fail()
	}

	// Scan the shared key prefix through the transaction API.
	n = 0
	err = tx.ScanPrefix(ctx, []byte("test"), func(key, value []byte) error {
		n++
		return nil
	})

	// Verify the prefix scan visits all three entries without error.
	if err != nil {
		t.Fatal(err.Error())
	}
	if n != 3 {
		t.Fail()
	}

	// Discard the completed kvfile read transaction.
	tx.Discard()
}

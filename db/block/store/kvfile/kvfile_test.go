package block_store_kvfile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// failingReaderAt serves reads from data until fail is set.
type failingReaderAt struct {
	data []byte
	fail bool
}

// errRead is the read error returned once fail is set.
var errRead = errors.New("read failed")

// ReadAt reads from data, or returns errRead once fail is set.
func (r *failingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if r.fail {
		return 0, errRead
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}

// TestStatBlockReportsReadError tests that StatBlock returns a read error
// instead of reporting the block as missing.
func TestStatBlockReportsReadError(t *testing.T) {
	// Build the block reference and its kvfile key.
	ctx := context.Background()
	kvkey := store_kvkey.NewDefaultKVKey()
	data := []byte("block data")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rm, err := ref.MarshalKey()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Write a kvfile holding the block.
	var buf bytes.Buffer
	err = kvfile.Write(&buf, [][]byte{kvkey.GetBlockKey(rm)}, func(wr io.Writer, key []byte) (uint64, error) {
		n, err := wr.Write(data)
		return uint64(n), err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open the kvfile as a block store, then fail every later read.
	rd := &failingReaderAt{data: buf.Bytes()}
	kvr, err := kvfile.BuildReader(rd, uint64(buf.Len()))
	if err != nil {
		t.Fatal(err.Error())
	}
	store := NewKvfileBlock(ctx, kvkey, kvr)
	rd.fail = true

	// Verify the read error reaches the caller.
	stat, err := store.StatBlock(ctx, ref)
	if !errors.Is(err, errRead) {
		t.Fatalf("StatBlock: stat %v err %v", stat, err)
	}
}

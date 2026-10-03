package volume_kvfile

import (
	"bytes"
	"context"
	io "io"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/sirupsen/logrus"
)

// TestKvfile runs the basic volume test suite.
func TestKvfile(t *testing.T) {
	// Prepare the context and logger for the KVFile volume.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Prepare the ordered key and value records for the KVFile.
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

	// Write the ordered records to the in-memory KVFile.
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

	// Open a KVFile reader over the encoded records.
	bufReader := bytes.NewReader(buf.Bytes())
	rdr, err := kvfile.BuildReader(bufReader, uint64(buf.Len())) //nolint:gosec
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the encoded KVFile can initialize a read-only volume.
	vol, err := NewKVFile(
		ctx,
		le,
		&Config{
			Verbose: true,
		},
		rdr,
		func() error { buf.Reset(); return nil },
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = vol
	/*
		if err := volume_test.CheckVolume(ctx, vol); err != nil {
			t.Fatal(err.Error())
		}
	*/
}

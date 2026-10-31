package block_store_ftps

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// TestClient walks the object contract over explicit TLS: parallel writes
// into new directories, whole and range reads, listing, deletes, a PackStore
// round trip, and the netrc credentials.
func TestClient(t *testing.T) {
	ctx := t.Context()
	addr, tlsConf := startTestServer(t)

	// Read the credentials from a netrc file.
	netrc := filepath.Join(t.TempDir(), "netrc")
	conf := "machine other login x password y\nmachine 127.0.0.1 login user password pass\n"
	if err := os.WriteFile(netrc, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	user, pass, err := ReadNetrc(netrc, "127.0.0.1")
	if err != nil || user != "user" || pass != "pass" {
		t.Fatalf("netrc = %q, %q, %v; want the 127.0.0.1 entry", user, pass, err)
	}
	client := NewClient(Config{Addr: addr, User: user, Password: pass, TLSConfig: tlsConf, Conns: 3})
	t.Cleanup(client.Close)

	// Write eight objects in parallel into directories that do not exist.
	eg, egCtx := errgroup.WithContext(ctx)
	for i := range 8 {
		eg.Go(func() error {
			data := bytes.Repeat([]byte{byte('a' + i)}, 1000+i)
			return client.PutObject(egCtx, "b", "d/"+strconv.Itoa(i%2)+"/o"+strconv.Itoa(i), data, "")
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// Replace one object and read it whole.
	if err := client.PutObject(ctx, "b", "d/0/o0", []byte("0123456789"), ""); err != nil {
		t.Fatal(err)
	}
	body, err := client.GetObject(ctx, "b", "d/0/o0")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "0123456789" {
		t.Fatalf("object = %q; want the replacement", data)
	}

	// Read ranges inside, across and past the end, on one connection after
	// another.
	for _, tc := range []struct {
		off  int64
		n    int
		want string
	}{{2, 3, "234"}, {7, 10, "789"}, {12, 4, ""}, {0, 1, "0"}} {
		got, err := client.GetObjectRange(ctx, "b", "d/0/o0", tc.off, tc.n)
		if err != nil || string(got) != tc.want {
			t.Fatalf("range %d+%d = %q, %v; want %q", tc.off, tc.n, got, err, tc.want)
		}
	}

	// Report a missing object as not found.
	if _, err := client.GetObjectRange(ctx, "b", "d/0/missing", 0, 4); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("missing range = %v; want ErrNotFound", err)
	}

	// List a key prefix and a directory, then delete and list again.
	var keys []string
	list := func(prefix string) []string {
		keys = keys[:0]
		err := client.ListObjects(ctx, "b", prefix, func(key string, _ int64) error {
			keys = append(keys, key)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return keys
	}
	if got := list("d/1/"); !slices.Equal(got, []string{"d/1/o1", "d/1/o3", "d/1/o5", "d/1/o7"}) {
		t.Fatalf("list d/1/ = %v", got)
	}
	if got := list("d/0/o"); len(got) != 4 || got[0] != "d/0/o0" {
		t.Fatalf("list d/0/o = %v", got)
	}
	if err := client.DeleteObject(ctx, "b", "d/1/o1"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteObject(ctx, "b", "d/1/o1"); err != nil {
		t.Fatalf("delete missing = %v; want nil", err)
	}
	if got := list("d/"); len(got) != 7 || got[0] != "d/0/o0" || got[4] != "d/1/o3" {
		t.Fatalf("list d/ = %v", got)
	}
	if got := list("none/"); len(got) != 0 {
		t.Fatalf("list none/ = %v; want nothing", got)
	}

	// Write a block through a PackStore and read it from a fresh one.
	le := logrus.NewEntry(logrus.New())
	opts := &block_store_s3.PackStoreOptions{KeepPacks: true}
	writer := block_store_s3.NewPackStore(le, client, "b", "packs/", opts)
	t.Cleanup(writer.Close)
	ref, _, err := writer.PutBlock(ctx, []byte("block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	reader := block_store_s3.NewPackStore(le, client, "b", "packs/", opts)
	t.Cleanup(reader.Close)
	stored, err := reader.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || string(stored.Data) != "block" {
		t.Fatalf("stored = %v, %v; want the block", stored, err)
	}
}

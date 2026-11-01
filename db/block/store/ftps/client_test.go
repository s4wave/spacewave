package block_store_ftps

import (
	"bytes"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

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
	logins, err := ReadNetrc(netrc, "127.0.0.1")
	if err != nil || len(logins) != 1 || logins[0] != (Login{User: "user", Password: "pass"}) {
		t.Fatalf("netrc = %v, %v; want the 127.0.0.1 entry", len(logins), err)
	}
	client := NewClient(Config{Addr: addr, Logins: logins, TLSConfig: tlsConf, MaxSessions: 4})
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

// TestClientPool spreads sessions over two logins capped at three sessions
// each, spares included, against a server that ends the session of every
// download closed before its end. It checks that range reads that spend
// sessions return the right bytes, that no login exceeds its cap, that a
// spare keeps the next read from waiting on a dial, and that a range reaching
// the end of the object keeps its session and dials nothing.
func TestClientPool(t *testing.T) {
	ctx := t.Context()
	addr, tlsConf, srv := startCountingServer(t)

	// Read two logins of the host from the netrc file.
	netrc := filepath.Join(t.TempDir(), "netrc")
	conf := "machine 127.0.0.1 login user1 password pass\nmachine other login x password y\nmachine 127.0.0.1 login user2 password pass\n"
	if err := os.WriteFile(netrc, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	logins, err := ReadNetrc(netrc, "127.0.0.1")
	if err != nil || len(logins) != 2 || logins[0].User != "user1" || logins[1].User != "user2" {
		t.Fatalf("logins = %v, %v; want user1 and user2 in file order", logins, err)
	}
	srv.mtx.Lock()
	srv.spendPartial = true
	srv.mtx.Unlock()

	// Write objects and read ranges of them from many goroutines.
	client := NewClient(Config{Addr: addr, Logins: logins, TLSConfig: tlsConf, ConnsPerLogin: 3, MaxSessions: 5})
	t.Cleanup(client.Close)
	if client.Conns() != 4 {
		t.Fatalf("conns = %d; want two slots on each login", client.Conns())
	}
	for i := range 6 {
		if err := client.PutObject(ctx, "b", "o"+strconv.Itoa(i), bytes.Repeat([]byte{byte('a' + i)}, 5000), ""); err != nil {
			t.Fatal(err)
		}
	}
	eg, egCtx := errgroup.WithContext(ctx)
	for g := range 8 {
		eg.Go(func() error {
			for n := range 6 {
				i := (g + n) % 6
				got, err := client.GetObjectRange(egCtx, "b", "o"+strconv.Itoa(i), 100, 50)
				if err != nil || !bytes.Equal(got, bytes.Repeat([]byte{byte('a' + i)}, 50)) {
					return errors.Errorf("range of o%d = %d bytes, %v", i, len(got), err)
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
	srv.mtx.Lock()
	peaks := []int{srv.peak["user1"], srv.peak["user2"]}
	used := []int{srv.logins["user1"], srv.logins["user2"]}
	totalPeak := srv.totalPeak
	srv.mtx.Unlock()
	if totalPeak > 5 {
		t.Fatalf("peak %d sessions from the client; want at most 5", totalPeak)
	}
	for i, peak := range peaks {
		if peak > 3 || used[i] == 0 {
			t.Fatalf("login %d: peak %d sessions, %d logins; want use within the cap of 3", i+1, peak, used[i])
		}
	}

	// Reads to the end of the object keep the session and dial no spare.
	one := NewClient(Config{Addr: addr, Logins: logins[:1], TLSConfig: tlsConf, MaxSessions: 2})
	t.Cleanup(one.Close)
	if err := one.PutObject(ctx, "b", "tail", []byte("0123456789"), ""); err != nil {
		t.Fatal(err)
	}
	before := srvLogins(srv, "user1")
	for range 3 {
		if got, err := one.GetObjectRange(ctx, "b", "tail", 6, 20); err != nil || string(got) != "6789" {
			t.Fatalf("tail = %q, %v", got, err)
		}
	}
	if n := srvLogins(srv, "user1") - before; n > 1 {
		t.Fatalf("tail reads logged in %d times; want at most the pool's first session", n)
	}

	// With slow dials and slow reads, the spare dialed during a read hides
	// the dial of the next one.
	if err := one.PutObject(ctx, "b", "big", bytes.Repeat([]byte{'x'}, 5000), ""); err != nil {
		t.Fatal(err)
	}
	srv.mtx.Lock()
	srv.dialDelay, srv.readDelay = 400*time.Millisecond, 200*time.Millisecond
	srv.mtx.Unlock()
	one.Close()
	start := time.Now()
	for range 4 {
		if got, err := one.GetObjectRange(ctx, "b", "big", 10, 20); err != nil || len(got) != 20 {
			t.Fatalf("range = %d bytes, %v", len(got), err)
		}
	}
	// Each read takes two delayed server reads, 0.4s. Without spares: 4 dials
	// and 4 reads, 3.2s. With: one dial, 4 reads, 2s.
	if took := time.Since(start); took > 2600*time.Millisecond {
		t.Fatalf("4 spending reads took %s; want the spare to hide the dials", took)
	}
}

// srvLogins returns the logins the server has seen for user.
func srvLogins(srv *testServer, user string) int {
	srv.mtx.Lock()
	defer srv.mtx.Unlock()
	return srv.logins[user]
}

// TestIsFull recognizes the server's session limit replies at the greeting
// and at the login, and no other login failure.
func TestIsFull(t *testing.T) {
	full := &textproto.Error{Code: 530, Msg: "Sorry, the maximum number of connections (10) for your host are already connected."}
	if !isFull(errors.Wrap(full, "connect")) {
		t.Fatal("a 530 maximum number reply is not full")
	}
	if isFull(&textproto.Error{Code: 530, Msg: "Login incorrect."}) || isFull(errors.New("x")) {
		t.Fatal("a bad login is not full")
	}
}

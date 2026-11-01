package block_store_sftp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/crypto/ssh"
)

// testServer is an in-process SSH server with an SFTP subsystem on the real
// filesystem, that accepts the login user and pass.
type testServer struct {
	addr string
	key  ssh.Signer
	mu   sync.Mutex
	// conns are the open connections, which kill drops.
	conns []net.Conn
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	conf := &ssh.ServerConfig{
		PasswordCallback: func(m ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if m.User() == "user" && string(pw) == "pass" {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	conf.AddHostKey(signer)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	s := &testServer{addr: lis.Addr().String(), key: signer}
	go func() {
		for {
			nc, err := lis.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, nc)
			s.mu.Unlock()
			go serve(nc, conf)
		}
	}()
	return s
}

func serve(nc net.Conn, conf *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nc, conf)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		ch, creqs, err := nch.Accept()
		if err != nil {
			return
		}
		go func() {
			for r := range creqs {
				ok := r.Type == "subsystem"
				_ = r.Reply(ok, nil)
				if ok {
					go func() {
						srv, err := sftp.NewServer(ch)
						if err == nil {
							_ = srv.Serve()
						}
						_ = ch.Close()
					}()
				}
			}
		}()
	}
}

// kill drops every open connection.
func (s *testServer) kill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, nc := range s.conns {
		_ = nc.Close()
	}
	s.conns = nil
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	srv := startTestServer(t)
	bucket := t.TempDir()
	c, err := NewClient(Config{Addr: srv.addr, User: "user", Password: "pass", HostKey: string(ssh.MarshalAuthorizedKey(srv.key.PublicKey())), Conns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Write, read ranges exactly and past the end, and list in name order.
	data := bytes.Repeat([]byte("0123456789"), 100)
	for _, key := range []string{"world/packs/b.pack", "world/packs/a.pack", "other/x.pack"} {
		if err := c.PutObject(ctx, bucket, key, data, ""); err != nil {
			t.Fatal(err)
		}
	}
	for range 4 {
		got, err := c.GetObjectRange(ctx, bucket, "world/packs/a.pack", 995, 100)
		if err != nil || string(got) != "56789" {
			t.Fatalf("range past the end: %q, %v", got, err)
		}
		got, err = c.GetObjectRange(ctx, bucket, "world/packs/a.pack", 3, 4)
		if err != nil || string(got) != "3456" {
			t.Fatalf("range: %q, %v", got, err)
		}
	}
	var keys []string
	if err := c.ListObjects(ctx, bucket, "world/", func(key string, size int64) error {
		keys = append(keys, key)
		if size != int64(len(data)) {
			t.Fatalf("size of %s is %d", key, size)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "world/packs/a.pack" || keys[1] != "world/packs/b.pack" {
		t.Fatalf("listed %v", keys)
	}

	// A missing object is ErrNotFound, and nothing is left under a temporary name.
	if _, err := c.GetObjectRange(ctx, bucket, "world/packs/none.pack", 0, 4); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("missing object: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(bucket, "world/packs")); len(ents) != 2 {
		t.Fatalf("directory holds %d entries", len(ents))
	}

	// Replace an object: the cached open file must not serve the old bytes.
	if err := c.PutObject(ctx, bucket, "world/packs/a.pack", []byte("new"), ""); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetObjectRange(ctx, bucket, "world/packs/a.pack", 0, 10); err != nil || string(got) != "new" {
		t.Fatalf("replaced object: %q, %v", got, err)
	}
	body, err := c.GetObject(ctx, bucket, "world/packs/b.pack")
	if err != nil {
		t.Fatal(err)
	}
	if all, _ := io.ReadAll(body); !bytes.Equal(all, data) {
		t.Fatal("whole object differs")
	}

	// Reads continue on fresh connections after the server drops them.
	srv.kill()
	for range 4 {
		if got, err := c.GetObjectRange(ctx, bucket, "world/packs/b.pack", 3, 4); err != nil || string(got) != "3456" {
			t.Fatalf("range after reconnect: %q, %v", got, err)
		}
	}

	// A wrong pin refuses the server.
	bad, err := NewClient(Config{Addr: srv.addr, User: "user", Password: "pass", HostKey: "SHA256:AAAA"})
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if _, err := bad.GetObjectRange(ctx, bucket, "world/packs/b.pack", 0, 1); err == nil {
		t.Fatal("a wrong host key pin was accepted")
	}

	// Delete.
	if err := c.DeleteObject(ctx, bucket, "world/packs/b.pack"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetObjectRange(ctx, bucket, "world/packs/b.pack", 0, 1); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("deleted object: %v", err)
	}
}

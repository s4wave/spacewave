package block_store_ftps

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// silentServer accepts connections and never answers, counting the ones
// open now and the most open at once. It drops its count of a connection
// before closing it, when it reads the client's close.
type silentServer struct {
	listener net.Listener
	mtx      sync.Mutex
	open     int
	peak     int
	accepted int
	closed   int
}

func newSilentServer(t *testing.T) *silentServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &silentServer{listener: l}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			s.mtx.Lock()
			s.open++
			s.accepted++
			s.peak = max(s.peak, s.open)
			s.mtx.Unlock()
			go func() {
				_, _ = io.Copy(io.Discard, conn)
				s.mtx.Lock()
				s.open--
				s.closed++
				s.mtx.Unlock()
				_ = conn.Close()
			}()
		}
	}()
	return s
}

func (s *silentServer) counts() (open, peak, accepted, closed int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.open, s.peak, s.accepted, s.closed
}

// TestCloseOnDoneSilentServer runs many timed out calls against a server
// that accepts and never answers: the sessions open at once never pass
// MaxSessions, spares included, and every timed out one is closed. With one
// session the client opens exactly one at a time and no spare.
func TestCloseOnDoneSilentServer(t *testing.T) {
	for _, sessions := range []int{1, 3} {
		t.Run(strconv.Itoa(sessions), func(t *testing.T) { testSilentServer(t, sessions) })
	}
}

// testSilentServer runs the calls with a client of sessions sessions.
func testSilentServer(t *testing.T, sessions int) {
	// Point the client at the silent server.
	srv := newSilentServer(t)
	client := NewClient(Config{
		Addr: srv.listener.Addr().String(), Logins: []Login{{User: "u", Password: "p"}},
		MaxSessions: sessions, ConnsPerLogin: sessions, CloseOnDone: true, NoFullRetry: true,
	})

	// Run four rounds of ten calls, each with a short deadline.
	for range 4 {
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				if _, err := client.GetObject(ctx, "b", "k"); err == nil {
					t.Error("a call to the silent server succeeded")
				}
			}()
		}
		wg.Wait()
	}

	// Every connection closes, and no more than the cap were open at once.
	deadline := time.Now().Add(5 * time.Second)
	for {
		open, peak, accepted, closed := srv.counts()
		if peak > sessions {
			t.Fatalf("peak %d open connections, cap %d", peak, sessions)
		}
		if open == 0 && accepted == closed && accepted > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("open %d, accepted %d, closed %d", open, accepted, closed)
		}
		time.Sleep(10 * time.Millisecond)
	}
	client.Close()
}

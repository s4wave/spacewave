//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
)

// This file uses only what ed9cfe587 has, so TestSlowOffSequence also runs
// there and its logged commands can be compared with a run here.

// trickle sends the object from rest in 4 KiB chunks 20 ms apart until the
// client closes the data connection or the object ends, closes the data
// connection, then answers on the control connection as shape says:
// "426", "226", "426+226", "none" (no reply: the NOOP's 200 comes first),
// or "end" (the server closes the control connection). A trickle that ran
// to its end answers 226 whatever the shape, but for "end".
func trickle(shape string) func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
	return func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
		// Trickle until a write fails.
		cut := false
		for off := rest; off < int64(len(m.data)); off += 4 << 10 {
			if _, err := dc.Write(m.data[off:min(off+4<<10, int64(len(m.data)))]); err != nil {
				cut = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = dc.Close()

		// Answer the close.
		if shape == "end" {
			_ = raw.Close()
			return
		}
		if !cut {
			shape = "226"
		}
		switch shape {
		case "426":
			_, _ = ctl.WriteString("426 Connection closed; transfer aborted.\r\n")
		case "226":
			_, _ = ctl.WriteString("226 Transfer complete\r\n")
		case "426+226":
			_, _ = ctl.WriteString("426 Connection closed; transfer aborted.\r\n226 Abort successful\r\n")
		}
		_ = ctl.Flush()
	}
}

// stage1Client returns a client with stage 1's settings: one session and
// one dial.
func stage1Client(t *testing.T, m *miniFTPS) *block_store_ftps.Client {
	c := block_store_ftps.NewClient(block_store_ftps.Config{
		Addr: m.ln.Addr().String(), TLSConfig: m.cliTLS,
		Logins:      []block_store_ftps.Login{{User: "u", Password: "p"}},
		MaxSessions: 1, ConnsPerLogin: 1, NoFullRetry: true, CloseOnDone: true, MaxDials: 1,
	})
	t.Cleanup(c.Close)
	return c
}

// TestSlowOffSequence reads a trickled pack, then a second, with the slow
// flags unset and with the client's GetObjectMax called directly, the path
// of the binary before the flags: the server sees the same commands.
func TestSlowOffSequence(t *testing.T) {
	// cmds runs read on a fresh server and returns every command it saw.
	cmds := func(read func(m *miniFTPS)) []string {
		m := newMini(t)
		m.onRetr = trickle("426")
		read(m)
		m.mtx.Lock()
		defer m.mtx.Unlock()
		return append([]string(nil), m.cmds...)
	}
	off := cmds(func(m *miniFTPS) {
		f := newFetcher(stage1Client(t, m), "b", fetchOptions{Timeout: 3 * time.Second, CapBytes: 1 << 40, StopOnDrop: true}, 0)
		for _, key := range []string{"k1", "k2"} {
			if _, err := f.whole(context.Background(), key, uint64(len(m.data))); err != nil {
				t.Fatal(err)
			}
		}
	})
	direct := cmds(func(m *miniFTPS) {
		c := stage1Client(t, m)
		for _, key := range []string{"k1", "k2"} {
			if _, err := c.GetObjectMax(context.Background(), "b", key, int64(len(m.data))); err != nil {
				t.Fatal(err)
			}
		}
	})
	if strings.Join(off, " ") != strings.Join(direct, " ") || strings.Contains(strings.Join(off, " "), "NOOP") {
		t.Fatalf("flags unset %v, direct %v", off, direct)
	}
	t.Logf("commands %s", strings.Join(off, " "))
}

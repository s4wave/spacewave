//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
)

// The slow tests serve 64 KiB in 4 KiB chunks 20 ms apart, about 200 KB/s,
// against a 1 MB/s threshold after a 100 ms grace; a trickle that runs to
// its end takes about 320 ms.
const (
	slowTestRate  = 1 << 20
	slowTestGrace = 100 * time.Millisecond
)

// sequence runs the RETR handlers in order, the last for any later RETR.
func sequence(hs ...func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64)) func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
	var mtx sync.Mutex
	var n int
	return func(m *miniFTPS, ctl *bufio.Writer, raw net.Conn, dc net.Conn, rest int64) {
		mtx.Lock()
		h := hs[min(n, len(hs)-1)]
		n++
		mtx.Unlock()
		h(m, ctl, raw, dc, rest)
	}
}

// slowFetcher returns a fetcher of c with the slow check on and reasks
// re-asks, logging to log, with retries unless stage1 asks for stage 1's
// settings (no retries, --stop-on-drop).
func slowFetcher(c objectReader, reasks int, stage1 bool, log *bytes.Buffer) *fetcher {
	o := fetchOptions{Timeout: 3 * time.Second, Retries: 3, Backoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, CapBytes: 1 << 40, Log: log,
		SlowRate: slowTestRate, SlowGrace: slowTestGrace, SlowReasks: reasks}
	if stage1 {
		o.Retries, o.StopOnDrop = 0, true
	}
	return newFetcher(c, "b", o, 0)
}

// cmdsAfter returns the commands the server saw from the first RETR on.
func (m *miniFTPS) cmdsAfter() []string {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	for i, c := range m.cmds {
		if c == "RETR" {
			return append([]string(nil), m.cmds[i:]...)
		}
	}
	return nil
}

// TestSlowReplyShapes trickles the first RETR, which the client abandons,
// answering its close with each reply shape a server may send: the session
// is kept, in step, and the re-ask and a second pack on it succeed, under
// stage 1's settings (one session, one dial, no retries, --stop-on-drop).
func TestSlowReplyShapes(t *testing.T) {
	for _, shape := range []string{"426", "226", "426+226"} {
		t.Run(shape, func(t *testing.T) {
			// Trickle the first RETR, serve the rest at once.
			m := newMini(t)
			m.onRetr = sequence(trickle(shape), okRetr)
			var log bytes.Buffer
			f := slowFetcher(stage1Client(t, m), 3, true, &log)

			// The pack, then a second on the same session.
			for _, key := range []string{"k1", "k2"} {
				data, err := f.whole(context.Background(), key, uint64(len(m.data)))
				if err != nil || !bytes.Equal(data, m.data) {
					t.Fatalf("%s: err %v, %d bytes; log %q", key, err, len(data), log.String())
				}
			}

			// One login, three RETRs, one NOOP right after the first, one
			// re-ask line and no stop.
			_, logins, _, _ := m.counts()
			cmds := m.cmdsAfter()
			if logins != 1 || m.retrs() != 3 || len(cmds) < 2 || cmds[1] != "NOOP" || strings.Count(strings.Join(cmds, " "), "NOOP") != 1 {
				t.Fatalf("logins %d, retrs %d, cmds %v", logins, m.retrs(), cmds)
			}
			if strings.Count(log.String(), "slow re-ask 1/3 pack k1 bytes ") != 1 || !strings.Contains(log.String(), " close "+shape+"\n") || !strings.Contains(log.String(), "slow re-ask done pack k1 ask 2 bytes ") || strings.Contains(log.String(), "stop: ") {
				t.Fatalf("log %q", log.String())
			}
		})
	}
}

// TestSlowReaskThenFast trickles the first RETR and answers its close with
// 426, then serves the re-ask at once: the pack is read whole on one
// session, charged against the cap once.
func TestSlowReaskThenFast(t *testing.T) {
	m := newMini(t)
	m.onRetr = sequence(trickle("426"), okRetr)
	var log bytes.Buffer
	f := slowFetcher(m.client(t, 1, 0), 3, false, &log)
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	if err != nil || !bytes.Equal(data, m.data) {
		t.Fatalf("err %v, %d bytes", err, len(data))
	}
	_, logins, _, _ := m.counts()
	if logins != 1 || m.retrs() != 2 || f.fetched.Load() != uint64(len(m.data)) || f.requests.Load() != 2 {
		t.Fatalf("logins %d, retrs %d, fetched %d, requests %d", logins, m.retrs(), f.fetched.Load(), f.requests.Load())
	}
	l := log.String()
	if !strings.HasPrefix(l, "slow re-ask 1/3 pack k bytes ") || !strings.Contains(l, " close 426\n") || !strings.Contains(l, fmt.Sprintf("slow re-ask done pack k ask 2 bytes %d ms ", len(m.data))) {
		t.Fatalf("log %q", l)
	}
}

// TestSlowSessionEnded trickles the first RETR and closes the control
// connection at the client's close, under stage 1's settings: the run stops
// at once with the slow-close stop, not a drop, with no redial, one login
// and one RETR, and the next pack is not requested.
func TestSlowSessionEnded(t *testing.T) {
	m := newMini(t)
	m.onRetr = trickle("end")
	var log bytes.Buffer
	f := slowFetcher(stage1Client(t, m), 3, true, &log)
	start := time.Now()
	_, err1 := f.whole(context.Background(), "k1", uint64(len(m.data)))
	took := time.Since(start)
	_, err2 := f.whole(context.Background(), "k2", uint64(len(m.data)))

	// A hard stop of its own kind, promptly.
	var hard *hardStopError
	if !errors.As(err1, &hard) || !errors.Is(err1, block_store_ftps.ErrSlowEnded) || !errors.Is(err2, errHardStopped) || took > 2*time.Second {
		t.Fatalf("first %v after %v, second %v", err1, took, err2)
	}
	l := log.String()
	if strings.Count(l, "stop: slow close ended the session: ") != 1 || strings.Contains(l, "stop: connection dropped") || strings.Contains(l, "redial") || strings.Contains(l, "re-ask") {
		t.Fatalf("log %q", l)
	}
	if _, logins, _, _ := m.counts(); logins != 1 || m.retrs() != 1 {
		t.Fatalf("logins %d, retrs %d", logins, m.retrs())
	}
}

// TestSlowNoCloseReply trickles the first RETR and sends no reply to its
// close, so the NOOP's 200 comes first and the session cannot be shown in
// step: it is ended, and with retries the pack is read on a new session.
func TestSlowNoCloseReply(t *testing.T) {
	m := newMini(t)
	m.onRetr = sequence(trickle("none"), okRetr)
	var log bytes.Buffer
	f := slowFetcher(m.client(t, 1, 0), 3, false, &log)
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	if err != nil || !bytes.Equal(data, m.data) {
		t.Fatalf("err %v, %d bytes", err, len(data))
	}
	if _, logins, _, _ := m.counts(); logins != 2 || m.retrs() != 2 || strings.Contains(log.String(), "re-ask") {
		t.Fatalf("logins %d, retrs %d, log %q", logins, m.retrs(), log.String())
	}
}

// TestSlowReaskCap trickles every RETR: after the two re-asks allowed the
// last ask reads to the end at the trickle's rate, so the pack never fails
// for being slow, and the third ask is logged as the one that read it.
func TestSlowReaskCap(t *testing.T) {
	m := newMini(t)
	m.onRetr = trickle("426")
	var log bytes.Buffer
	f := slowFetcher(m.client(t, 1, 0), 2, false, &log)
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	if err != nil || !bytes.Equal(data, m.data) {
		t.Fatalf("err %v, %d bytes", err, len(data))
	}
	l := log.String()
	if _, logins, _, _ := m.counts(); logins != 1 || m.retrs() != 3 || strings.Count(l, "slow re-ask ") != 3 || !strings.Contains(l, "slow re-ask 2/2 pack k bytes ") || !strings.Contains(l, fmt.Sprintf("slow re-ask done pack k ask 3 bytes %d ms ", len(m.data))) {
		t.Fatalf("logins %d, retrs %d, log %q", logins, m.retrs(), l)
	}
}

// TestSlowInsideGrace trickles a RETR that ends before a 1 s grace: the
// rate is never checked, and the pack is read on one RETR with no NOOP.
func TestSlowInsideGrace(t *testing.T) {
	m := newMini(t)
	m.onRetr = trickle("426")
	var log bytes.Buffer
	f := slowFetcher(m.client(t, 1, 0), 3, false, &log)
	f.opts.SlowGrace = time.Second
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	if err != nil || !bytes.Equal(data, m.data) {
		t.Fatalf("err %v, %d bytes", err, len(data))
	}
	if cmds := m.cmdsAfter(); m.retrs() != 1 || strings.Contains(strings.Join(cmds, " "), "NOOP") || log.Len() != 0 {
		t.Fatalf("retrs %d, cmds %v, log %q", m.retrs(), cmds, log.String())
	}
}

// TestSlowKeptLongReask keeps a session after a 426 close under a 100 ms
// SlowCloseWait, then re-asks the pack past that bound: two requests a
// second hold the re-ask until about 500 ms, and the re-ask, the last ask,
// trickles about 320 ms to its end. The control connection's bound must be
// cleared, so the pack completes on one login with no drop and no ended
// session.
func TestSlowKeptLongReask(t *testing.T) {
	// Trickle every RETR to a client with a 100 ms close bound.
	m := newMini(t)
	m.onRetr = trickle("426")
	c := block_store_ftps.NewClient(block_store_ftps.Config{
		Addr: m.ln.Addr().String(), TLSConfig: m.cliTLS,
		Logins:      []block_store_ftps.Login{{User: "u", Password: "p"}},
		MaxSessions: 1, ConnsPerLogin: 1, NoFullRetry: true, CloseOnDone: true, MaxDials: 1,
		SlowCloseWait: 100 * time.Millisecond,
	})
	t.Cleanup(c.Close)
	var log bytes.Buffer
	f := newFetcher(c, "b", fetchOptions{Timeout: 3 * time.Second, CapBytes: 1 << 40, Log: &log, StopOnDrop: true, RequestsPerSec: 2,
		SlowRate: slowTestRate, SlowGrace: slowTestGrace, SlowReasks: 1}, 0)

	// The pack completes on one session, past the bound.
	start := time.Now()
	data, err := f.whole(context.Background(), "k", uint64(len(m.data)))
	took := time.Since(start)
	if err != nil || !bytes.Equal(data, m.data) {
		t.Fatalf("err %v, %d bytes; log %q", err, len(data), log.String())
	}
	l := log.String()
	if _, logins, _, _ := m.counts(); logins != 1 || m.retrs() != 2 || took < 400*time.Millisecond || strings.Count(l, "slow re-ask 1/1 pack k bytes ") != 1 || !strings.Contains(l, "slow re-ask done pack k ask 2 bytes ") || strings.Contains(l, "stop: ") {
		t.Fatalf("logins %d, retrs %d, took %v, log %q", logins, m.retrs(), took, l)
	}
}

//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// termChildEnv carries the command's flags, split by termArgSep, into the
// child process TestRescueTermLedger starts: there the test runs main.
const (
	termChildEnv = "SPACEWAVE_RESCUE_TERM_CHILD_ARGS"
	termArgSep   = "\x1f"
)

// TestRescueTermLedger runs the command through main in a child process, the
// production path with its signal handler, against miniFTPS serving five
// packs on one session. The server serves the first three at once and
// stalls the fourth after its first bytes; the test sends the child SIGTERM
// once the fourth RETR is in flight. After the child exits, the ledger's p
// lines must count exactly the packs whose wanted blocks are all in --out.
// The child trusts miniFTPS's certificate through SSL_CERT_FILE, which Go
// reads on Linux.
func TestRescueTermLedger(t *testing.T) {
	// Run main in the child.
	if a := os.Getenv(termChildEnv); a != "" {
		os.Args = append([]string{"spacewave-world-rescue"}, strings.Split(a, termArgSep)...)
		main()
		return
	}

	// Write five packs, the most wanted first in the plan's order.
	const served = 3
	src := newSource(t)
	var packs [][]*block.BlockRef
	var refs []*block.BlockRef
	for i := range 5 {
		p := src.pack(12 - i)
		packs = append(packs, p)
		refs = append(refs, p...)
	}
	src.close()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	list := writeList(t, dir, refs)
	index := termIndex(t, src, filepath.Join(dir, "index.db"))

	// Serve the packs from the source directory, stalling the fourth RETR
	// after its first bytes until the child has exited.
	m := newMini(t)
	release := make(chan struct{})
	inFlight := make(chan struct{})
	var once sync.Once
	var retrs int
	var mtx sync.Mutex
	m.object = func(name string) ([]byte, bool) {
		body, err := os.ReadFile(filepath.Join(src.dir, filepath.FromSlash(strings.TrimPrefix(name, "/"))))
		return body, err == nil
	}
	m.onObject = func(_ *miniFTPS, ctl *bufio.Writer, dc net.Conn, _ string, body []byte, rest int64) {
		mtx.Lock()
		retrs++
		n := retrs
		mtx.Unlock()
		body = body[min(rest, int64(len(body))):]
		if n == served+1 {
			_, _ = dc.Write(body[:min(512, len(body))])
			once.Do(func() { close(inFlight) })
			<-release
			_ = dc.Close()
			return
		}
		_, _ = dc.Write(body)
		_ = dc.Close()
		_, _ = ctl.WriteString("226 done\r\n")
		_ = ctl.Flush()
	}

	// Write the netrc and the certificate the child trusts.
	netrc := filepath.Join(dir, "netrc")
	cert := filepath.Join(dir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.tlsConf.Certificates[0].Certificate[0]})
	if err := os.WriteFile(netrc, []byte("machine 127.0.0.1 login u password p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cert, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	// Start the child on stage 1's kind of settings: one session, no
	// retries, --stop-on-drop, the default flush interval.
	flags := []string{
		"--refs", list, "--index", index, "--index-scratch", filepath.Join(dir, "view"),
		"--store-id", testStoreID, "--addr", m.ln.Addr().String(), "--netrc", netrc, "--out", out,
		"--sessions", "1", "--max-bytes", strconv.Itoa(1 << 30), "--disk-floor-gb", "0",
		"--requests-per-sec", "0", "--bytes-per-sec", "0", "--retries", "0", "--stop-on-drop",
		"--timeout", "30s",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRescueTermLedger$", "-test.count=1")
	cmd.Env = append(os.Environ(), termChildEnv+"="+strings.Join(flags, termArgSep), "SSL_CERT_FILE="+cert, "SSL_CERT_DIR="+dir)
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stderr, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer close(release)

	// TERM the child once the fourth pack is in flight, and wait for it.
	select {
	case <-inFlight:
	case err := <-exited:
		t.Fatalf("child exited before the fourth RETR: %v\n%s", err, stderr.String())
	case <-ctx.Done():
		t.Fatalf("no fourth RETR\n%s", stderr.String())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	var err error
	select {
	case err = <-exited:
	case <-ctx.Done():
		t.Fatalf("child did not exit after SIGTERM\n%s", stderr.String())
	}
	code := 0
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	// Count the ledger's p lines and the packs whole in the output.
	pLines := len(packLines(t, out))
	onDisk := 0
	for _, p := range packs {
		if checkOutput(t, out, p) == len(p) {
			onDisk++
		}
	}
	t.Logf("child exit %d, p lines %d, packs on disk %d, served %d", code, pLines, onDisk, served)
	if pLines != onDisk {
		t.Fatalf("p lines %d, packs on disk %d\n%s", pLines, onDisk, stderr.String())
	}
	if pLines != served {
		t.Fatalf("p lines and packs on disk %d, want the %d served before the TERM\n%s", pLines, served, stderr.String())
	}
}

// termIndex copies src's index rows into a SQLite pack index file at path,
// as the command opens it, and returns path.
func termIndex(t *testing.T, src *source, path string) string {
	t.Helper()
	ctx := context.Background()
	w, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, defaultIndexTable, store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	rtx, err := src.kv.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	wtx, err := w.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Discard()
	err = rtx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		return wtx.Set(ctx, bytes.Clone(key), bytes.Clone(value))
	})
	if err == nil {
		err = wtx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

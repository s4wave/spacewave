package git_lfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// memStore is a Store over a map that counts Put calls and can fail them.
type memStore struct {
	// objects maps oid to content.
	objects map[string][]byte
	// puts counts Put calls.
	puts int
	// putErr fails every Put when set.
	putErr error
}

func (s *memStore) Has(_ context.Context, oid string, size int64) (bool, error) {
	data, ok := s.objects[oid]
	return ok && int64(len(data)) == size, nil
}

func (s *memStore) Put(_ context.Context, oid string, _ int64, rdr io.Reader) error {
	s.puts++
	if s.putErr != nil {
		return s.putErr
	}
	data, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}
	s.objects[oid] = data
	return nil
}

func (s *memStore) Get(_ context.Context, oid string, _ int64, w io.Writer) error {
	data, ok := s.objects[oid]
	if !ok {
		return errors.New("not found")
	}
	_, err := w.Write(data)
	return err
}

// testObject returns content and its oid.
func testObject(content string) ([]byte, string) {
	sum := sha256.Sum256([]byte(content))
	return []byte(content), hex.EncodeToString(sum[:])
}

// runAgent runs an Agent over store with the given request lines and returns
// the parsed response lines.
func runAgent(t *testing.T, store Store, tmpDir string, requests ...string) []*fastjson.Value {
	t.Helper()
	var out bytes.Buffer
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	if err := NewAgent(store, tmpDir).Run(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	var msgs []*fastjson.Value
	for line := range strings.Lines(out.String()) {
		v, err := fastjson.Parse(line)
		if err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		msgs = append(msgs, v)
	}
	return msgs
}

// completes returns the complete events among msgs.
func completes(msgs []*fastjson.Value) []*fastjson.Value {
	var out []*fastjson.Value
	for _, msg := range msgs {
		if string(msg.GetStringBytes("event")) == "complete" {
			out = append(out, msg)
		}
	}
	return out
}

// uploadRequest encodes an upload event.
func uploadRequest(oid string, size int, path string) string {
	return `{"event":"upload","oid":"` + oid + `","size":` + strconv.Itoa(size) + `,"path":` + strconv.Quote(path) + `}`
}

// downloadRequest encodes a download event.
func downloadRequest(oid string, size int) string {
	return `{"event":"download","oid":"` + oid + `","size":` + strconv.Itoa(size) + `}`
}

const (
	initRequest      = `{"event":"init","operation":"upload","remote":"origin","concurrent":true,"concurrenttransfers":8}`
	terminateRequest = `{"event":"terminate"}`
)

// TestAgentUploadDownload uploads an object, skips it on the second upload,
// and downloads it back into a verified file.
func TestAgentUploadDownload(t *testing.T) {
	dir := t.TempDir()
	data, oid := testObject("hello git-lfs")
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	store := &memStore{objects: map[string][]byte{}}
	tmpDir := filepath.Join(dir, "tmp")

	// The first upload stores the object and reports its progress; the
	// second finds it and skips the Put.
	msgs := runAgent(t, store, tmpDir,
		initRequest,
		uploadRequest(oid, len(data), src),
		uploadRequest(oid, len(data), src),
		terminateRequest,
	)
	if msgs[0].String() != "{}" {
		t.Fatalf("init reply = %s, want {}", msgs[0])
	}
	if got := msgs[1]; string(got.GetStringBytes("event")) != "progress" || got.GetInt64("bytesSoFar") != int64(len(data)) {
		t.Fatalf("upload progress = %s", got)
	}
	done := completes(msgs)
	if len(done) != 2 {
		t.Fatalf("got %d complete events, want 2", len(done))
	}
	for _, msg := range done {
		if msg.Exists("error") {
			t.Fatalf("upload failed: %s", msg)
		}
	}
	if store.puts != 1 {
		t.Fatalf("store got %d puts, want 1", store.puts)
	}
	if !bytes.Equal(store.objects[oid], data) {
		t.Fatalf("stored %q, want %q", store.objects[oid], data)
	}

	// The download lands in tmpDir with the stored bytes.
	done = completes(runAgent(t, store, tmpDir, initRequest, downloadRequest(oid, len(data)), terminateRequest))
	if len(done) != 1 || done[0].Exists("error") {
		t.Fatalf("download complete = %v", done)
	}
	path := string(done[0].GetStringBytes("path"))
	if filepath.Dir(path) != tmpDir {
		t.Fatalf("download path %q is outside %q", path, tmpDir)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("downloaded %q, want %q", got, data)
	}
}

// TestAgentRejectsCorruptDownload reports a download whose bytes do not match
// the pointer as failed and removes its file.
func TestAgentRejectsCorruptDownload(t *testing.T) {
	tmpDir := t.TempDir()
	data, oid := testObject("expected")
	_, otherOid := testObject("different")
	store := &memStore{objects: map[string][]byte{
		oid:      []byte("tampered"),
		otherOid: data,
	}}

	// Right size, wrong hash; then a size that disagrees with the pointer.
	done := completes(runAgent(t, store, tmpDir,
		initRequest,
		downloadRequest(oid, len(data)),
		downloadRequest(otherOid, len(data)+1),
		terminateRequest,
	))
	if len(done) != 2 {
		t.Fatalf("got %d complete events, want 2", len(done))
	}
	want := []error{ErrOidMismatch, ErrSizeMismatch}
	for i, msg := range done {
		if got := string(msg.Get("error").GetStringBytes("message")); got != want[i].Error() {
			t.Fatalf("download %d error = %q, want %q", i, got, want[i])
		}
		if msg.Exists("path") {
			t.Fatalf("failed download %d reported a path", i)
		}
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed downloads left %d files", len(entries))
	}
}

// TestAgentReportsTransferErrors reports store failures and invalid oids to
// git-lfs and keeps serving later requests.
func TestAgentReportsTransferErrors(t *testing.T) {
	dir := t.TempDir()
	data, oid := testObject("payload")
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	store := &memStore{objects: map[string][]byte{}, putErr: errors.New("store offline")}

	done := completes(runAgent(t, store, dir,
		initRequest,
		uploadRequest("../escape", len(data), src),
		uploadRequest(oid, len(data), src),
		terminateRequest,
	))
	if len(done) != 2 {
		t.Fatalf("got %d complete events, want 2", len(done))
	}
	want := []string{ErrInvalidOid.Error(), "store offline"}
	for i, msg := range done {
		errObj := msg.Get("error")
		if errObj.GetInt("code") != transferErrorCode || string(errObj.GetStringBytes("message")) != want[i] {
			t.Fatalf("upload %d complete = %s, want error %q", i, msg, want[i])
		}
	}
}

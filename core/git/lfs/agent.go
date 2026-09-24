// Package git_lfs implements a git-lfs standalone custom transfer agent that
// stores Git LFS objects in a Store.
//
// git-lfs starts the agent once per transfer batch when the repository sets
// lfs.standalonetransferagent, and exchanges one JSON object per line on the
// agent's stdin and stdout. See
// https://github.com/git-lfs/git-lfs/blob/main/docs/custom-transfers.md.
package git_lfs

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// transferErrorCode is the error code reported for a failed transfer.
// git-lfs shows the message and does not interpret the code.
const transferErrorCode = 1

// Agent serves the git-lfs standalone custom transfer protocol over a Store.
type Agent struct {
	// store holds the objects.
	store Store
	// tmpDir receives downloaded files. It must be on the filesystem of the
	// git-lfs object store so git-lfs can rename them into place.
	tmpDir string
}

// NewAgent constructs an Agent that stores objects in store and writes
// downloads into tmpDir.
func NewAgent(store Store, tmpDir string) *Agent {
	return &Agent{store: store, tmpDir: tmpDir}
}

// Run reads protocol events from r and writes responses to w until git-lfs
// sends terminate or closes r. A failed transfer is reported to git-lfs and
// does not end Run; a malformed event or a write failure does.
func (a *Agent) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	rd := bufio.NewReader(r)
	out := &events{w: bufio.NewWriter(w)}
	var p fastjson.Parser
	for {
		// Read the next event line.
		line, err := rd.ReadBytes('\n')
		if len(line) == 0 && err == io.EOF {
			return nil
		}
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "read event")
		}
		v, err := p.ParseBytes(line)
		if err != nil {
			return errors.Wrap(err, "parse event")
		}

		// Dispatch the event. Strings are copied out of the parser because
		// the next ParseBytes reuses its memory.
		oid := string(v.GetStringBytes("oid"))
		size := v.GetInt64("size")
		switch event := string(v.GetStringBytes("event")); event {
		case "init":
			err = out.send(out.arena.NewObject())
		case "upload":
			err = a.upload(ctx, out, oid, size, string(v.GetStringBytes("path")))
		case "download":
			err = a.download(ctx, out, oid, size)
		case "terminate":
			return nil
		default:
			return errors.Errorf("unknown git-lfs event %q", event)
		}
		if err != nil {
			return err
		}
	}
}

// upload stores the file at path as oid unless the Store already holds it,
// then reports completion.
func (a *Agent) upload(ctx context.Context, out *events, oid string, size int64, path string) error {
	err := func() error {
		// Skip an object the Store already holds.
		if err := ValidateOid(oid); err != nil {
			return err
		}
		has, err := a.store.Has(ctx, oid, size)
		if err != nil {
			return err
		}
		if has {
			return nil
		}

		// Stream the file into the Store, reporting progress as it is read.
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return a.store.Put(ctx, oid, size, io.TeeReader(f, &progress{out: out, oid: oid}))
	}()
	return out.complete(oid, "", err)
}

// download fetches oid into a temporary file, verifies its size and hash, and
// reports the file's path.
func (a *Agent) download(ctx context.Context, out *events, oid string, size int64) error {
	path, err := func() (string, error) {
		// Create the temporary file beside the git-lfs object store.
		if err := ValidateOid(oid); err != nil {
			return "", err
		}
		if err := os.MkdirAll(a.tmpDir, 0o755); err != nil {
			return "", err
		}
		f, err := os.CreateTemp(a.tmpDir, oid+"-*")
		if err != nil {
			return "", err
		}
		path := f.Name()

		// Write the object while hashing it and reporting progress.
		hash := sha256.New()
		prog := &progress{out: out, oid: oid}
		err = a.store.Get(ctx, oid, size, io.MultiWriter(f, hash, prog))
		if cerr := f.Close(); err == nil {
			err = cerr
		}

		// Reject an object whose bytes do not match the pointer.
		if err == nil && prog.sofar != size {
			err = ErrSizeMismatch
		}
		if err == nil && hex.EncodeToString(hash.Sum(nil)) != oid {
			err = ErrOidMismatch
		}
		if err != nil {
			_ = os.Remove(path) // the transfer error is the one to report
			return "", err
		}
		return path, nil
	}()
	return out.complete(oid, path, err)
}

// ValidateOid checks that oid is a SHA-256 digest in lowercase hex, the form
// git-lfs uses, so it is safe to use as a path component.
func ValidateOid(oid string) error {
	if len(oid) != sha256.Size*2 {
		return ErrInvalidOid
	}
	for i := range len(oid) {
		c := oid[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidOid
		}
	}
	return nil
}

// events writes protocol messages to git-lfs, one JSON object per line.
type events struct {
	// w buffers output; every message is flushed when sent.
	w *bufio.Writer
	// arena allocates the values of one message and is reset after each.
	arena fastjson.Arena
	// buf holds the encoding of one message.
	buf []byte
}

// send writes v as one line, flushes it, and resets the arena.
func (e *events) send(v *fastjson.Value) error {
	e.buf = append(v.MarshalTo(e.buf[:0]), '\n')
	e.arena.Reset()
	if _, err := e.w.Write(e.buf); err != nil {
		return err
	}
	return e.w.Flush()
}

// complete reports the end of a transfer: its downloaded path when set, or
// transferErr when the transfer failed.
func (e *events) complete(oid, path string, transferErr error) error {
	msg := e.event("complete", oid)
	if path != "" {
		msg.Set("path", e.arena.NewString(path))
	}
	if transferErr != nil {
		errObj := e.arena.NewObject()
		errObj.Set("code", e.arena.NewNumberInt(transferErrorCode))
		errObj.Set("message", e.arena.NewString(transferErr.Error()))
		msg.Set("error", errObj)
	}
	return e.send(msg)
}

// event starts a message of the given event type for oid.
func (e *events) event(event, oid string) *fastjson.Value {
	msg := e.arena.NewObject()
	msg.Set("event", e.arena.NewString(event))
	msg.Set("oid", e.arena.NewString(oid))
	return msg
}

// progress reports the bytes written through it as progress events for one
// transfer.
type progress struct {
	// out receives the progress events.
	out *events
	// oid is the object being transferred.
	oid string
	// sofar counts the bytes transferred.
	sofar int64
}

// Write counts p and reports it to git-lfs.
func (p *progress) Write(b []byte) (int, error) {
	p.sofar += int64(len(b))
	msg := p.out.event("progress", p.oid)
	msg.Set("bytesSoFar", p.out.arena.NewNumberString(strconv.FormatInt(p.sofar, 10)))
	msg.Set("bytesSinceLast", p.out.arena.NewNumberInt(len(b)))
	if err := p.out.send(msg); err != nil {
		return 0, err
	}
	return len(b), nil
}

// _ is a type assertion
var _ io.Writer = ((*progress)(nil))

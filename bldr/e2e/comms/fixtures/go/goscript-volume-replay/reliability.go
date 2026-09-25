//go:build goscript

package goscript_volume_replay

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/pkg/errors"
	block "github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	"github.com/s4wave/spacewave/db/volume/logindex"
	"github.com/s4wave/spacewave/db/volume/paylog"
)

// The reliability workload repeats one step: put a block, then commit the
// step number as the head and the block's reference with write ordering.
// Every syncEvery steps a Sync makes the steps so far durable and the worker
// logs "synced <step>". A recovered volume must hold a prefix of the steps
// that reaches the last logged Sync, and every synced step's block.
const (
	// stepBlockSize is the payload length of each step's block.
	stepBlockSize = 16 << 10
	// syncEvery is the number of steps between Syncs.
	syncEvery = 4
	// maxSteps bounds a burst the harness fails to stop.
	maxSteps = 20000
	// reliabilityChannel is the BroadcastChannel the harness and the
	// worker coordinate on.
	reliabilityChannel = "volume-reliability"
)

// reliabilityReport is the result of one reliability mode.
type reliabilityReport struct {
	Mode string `json:"mode"`
	// Head is the last step the volume holds, -1 when empty.
	Head int `json:"head"`
	// Synced is the last step a Sync covered, -1 when none.
	Synced int `json:"synced"`
	// Errors are the operation errors the mode expected and observed.
	Errors []string `json:"errors,omitempty"`
}

// reliability runs one reliability mode, given as "<mode>:<name>[:<arg>]".
func reliability(ctx context.Context, spec string) (*reliabilityReport, error) {
	parts := strings.Split(spec, ":")
	mode, name := parts[0], parts[1]
	arg := -1
	if len(parts) > 2 {
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, err
		}
		arg = n
	}
	path := storageName + "/rel-" + name
	rep := &reliabilityReport{Mode: mode, Head: -1, Synced: -1}
	switch mode {
	case "burst":
		return rep, burst(ctx, path, rep)
	case "verify":
		return rep, verifyPath(ctx, path, arg, rep)
	case "quota":
		return rep, quota(ctx, path, rep)
	case "evict":
		return rep, evict(ctx, path, rep)
	case "write":
		return rep, write(ctx, path, arg, rep)
	case "wipe":
		return rep, device_opfs.Delete(path)
	case "reopen":
		return rep, reopen(ctx, path, arg, rep)
	case "hold":
		return rep, hold(ctx, path, rep)
	case "contend":
		return rep, contend(ctx, path, rep)
	}
	return nil, errors.Errorf("unknown reliability mode %q", mode)
}

// burst steps a fresh volume until the harness stops the worker.
func burst(ctx context.Context, path string, rep *reliabilityReport) error {
	if err := device_opfs.Delete(path); err != nil {
		return err
	}
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	defer closeVolume()
	for i := range maxSteps {
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			return err
		}
	}
	return errors.Errorf("burst reached %d steps without being stopped", maxSteps)
}

// verifyPath opens the volume at path and checks it against synced.
func verifyPath(ctx context.Context, path string, synced int, rep *reliabilityReport) error {
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	defer closeVolume()
	rep.Synced = synced
	rep.Head, err = verify(ctx, s, synced)
	return err
}

// quota steps a fresh volume until a write fails, checks that the synced
// steps stay readable, then waits for "space-freed" and checks that the
// volume takes and syncs writes again.
func quota(ctx context.Context, path string, rep *reliabilityReport) error {
	ch := listen()
	defer ch.close()
	if err := device_opfs.Delete(path); err != nil {
		return err
	}
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	defer closeVolume()

	// Fill until a write fails.
	i := 0
	for ; ; i++ {
		if i == maxSteps {
			return errors.Errorf("no quota error after %d steps", maxSteps)
		}
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			break
		}
	}
	if _, err := verify(ctx, s, rep.Synced); err != nil {
		return errors.Wrap(err, "after the quota error")
	}
	logf("quota-hit %d", rep.Synced)

	// Continue once the harness frees space.
	if err := ch.wait(ctx, "space-freed"); err != nil {
		return err
	}
	for j := range 2 * syncEvery {
		if err := stepAndSync(ctx, s, i+j, rep); err != nil {
			return errors.Wrap(err, "after space was freed")
		}
	}
	rep.Head, err = verify(ctx, s, rep.Synced)
	return err
}

// evict steps a fresh volume, waits while the harness clears the origin's
// storage, and records what the open volume does next. It leaves the volume
// open: Chromium's FileSystemSyncAccessHandle.close never returns after the
// origin is cleared under an open handle, so the harness closes the page and
// reopens the volume in a new one.
func evict(ctx context.Context, path string, rep *reliabilityReport) error {
	ch := listen()
	defer ch.close()
	if err := device_opfs.Delete(path); err != nil {
		return err
	}
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	for i := range 2 * syncEvery {
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			closeVolume()
			return err
		}
	}
	logf("evict-ready")
	if err := ch.wait(ctx, "evicted"); err != nil {
		closeVolume()
		return err
	}

	// The open volume must keep answering, with errors or without.
	if err := stepAndSync(ctx, s, 2*syncEvery, rep); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	rep.Head, err = verify(ctx, s, -1)
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	return nil
}

// write steps a fresh volume n times, syncs, and closes it.
func write(ctx context.Context, path string, n int, rep *reliabilityReport) error {
	if err := device_opfs.Delete(path); err != nil {
		return err
	}
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	defer closeVolume()
	for i := range n {
		if err := step(ctx, s, i); err != nil {
			return err
		}
	}
	if err := syncSteps(ctx, s, n-1, rep); err != nil {
		return err
	}
	rep.Head = n - 1
	return nil
}

// reopen opens the volume at path, which must hold head steps (any prefix
// when head is -2), then steps and syncs it further.
func reopen(ctx context.Context, path string, head int, rep *reliabilityReport) error {
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	defer closeVolume()
	got, err := verify(ctx, s, max(head, -1))
	if err != nil {
		return err
	}
	if head != -2 && got != head {
		return errors.Errorf("reopened at step %d, want %d", got, head)
	}
	rep.Synced = got
	for i := got + 1; i <= got+syncEvery; i++ {
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			return errors.Wrap(err, "after reopen")
		}
	}
	rep.Head, err = verify(ctx, s, rep.Synced)
	return err
}

// hold steps a fresh volume, announces "held", and keeps the volume open
// until a second opener announces "contended". It then closes the volume and
// announces "released".
func hold(ctx context.Context, path string, rep *reliabilityReport) error {
	ch := listen()
	defer ch.close()
	if err := device_opfs.Delete(path); err != nil {
		return err
	}
	s, closeVolume, err := openVolume(ctx, path)
	if err != nil {
		return err
	}
	for i := range syncEvery {
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			closeVolume()
			return err
		}
	}
	logf("held")
	if err := ch.wait(ctx, "contended"); err != nil {
		closeVolume()
		return err
	}

	// The holder keeps working while the other opener is refused.
	for i := syncEvery; i < 2*syncEvery; i++ {
		if err := stepAndSync(ctx, s, i, rep); err != nil {
			closeVolume()
			return errors.Wrap(err, "after the second open")
		}
	}
	rep.Head, err = verify(ctx, s, rep.Synced)
	closeVolume()
	if err != nil {
		return err
	}
	ch.post("released")
	return nil
}

// contend opens the volume a holder has open, which must fail, announces
// "contended", then opens it again once the holder announces "released".
func contend(ctx context.Context, path string, rep *reliabilityReport) error {
	ch := listen()
	defer ch.close()
	_, closeVolume, err := openVolume(ctx, path)
	if err == nil {
		closeVolume()
		return errors.New("opened a volume another worker holds")
	}
	rep.Errors = append(rep.Errors, err.Error())
	ch.post("contended")
	if err := ch.wait(ctx, "released"); err != nil {
		return err
	}
	return verifyPath(ctx, path, 2*syncEvery-1, rep)
}

// openVolume opens E1 on the OPFS directory at path.
func openVolume(ctx context.Context, path string) (*paylog.Store, func(), error) {
	dev, err := device_opfs.Open(path)
	if err != nil {
		return nil, nil, err
	}
	idx, err := logindex.Open(ctx, dev, logindex.Options{})
	if err != nil {
		_ = dev.Close()
		return nil, nil, err
	}
	s, err := paylog.Open(ctx, dev, idx)
	if err != nil {
		_ = dev.Close()
		return nil, nil, err
	}
	return s, func() {
		_ = s.Close()
		_ = dev.Close()
	}, nil
}

// stepAndSync runs step i and, every syncEvery steps, a Sync.
func stepAndSync(ctx context.Context, s *paylog.Store, i int, rep *reliabilityReport) error {
	if err := step(ctx, s, i); err != nil {
		return err
	}
	if (i+1)%syncEvery != 0 {
		return nil
	}
	return syncSteps(ctx, s, i, rep)
}

// syncSteps makes the steps through i durable and logs it.
func syncSteps(ctx context.Context, s *paylog.Store, i int, rep *reliabilityReport) error {
	if _, err := s.Sync(ctx); err != nil {
		return errors.Wrapf(err, "sync at step %d", i)
	}
	rep.Synced = i
	logf("synced %d", i)
	return nil
}

// step puts step i's block and commits i as the head with its reference.
func step(ctx context.Context, s *paylog.Store, i int) error {
	ref, _, err := s.PutBlock(ctx, payload(i), nil)
	if err != nil {
		return errors.Wrapf(err, "put block %d", i)
	}
	tx, err := s.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("head"), []byte(strconv.Itoa(i))); err != nil {
		return err
	}
	if err := tx.Set(ctx, refKey(i), []byte(ref.MarshalString())); err != nil {
		return err
	}
	if err := kvtx.CommitOrdered(ctx, tx); err != nil {
		return errors.Wrapf(err, "commit step %d", i)
	}
	return nil
}

// verify checks that s holds a prefix of the steps reaching at least synced,
// that every synced step's block is present, and that every present block is
// intact. It returns the last step held, -1 when none.
func verify(ctx context.Context, s *paylog.Store, synced int) (int, error) {
	tx, err := s.NewTransaction(ctx, false)
	if err != nil {
		return 0, err
	}
	defer tx.Discard()

	// Read the head.
	head := -1
	v, found, err := tx.Get(ctx, []byte("head"))
	if err != nil {
		return 0, err
	}
	if found {
		if head, err = strconv.Atoi(string(v)); err != nil {
			return 0, errors.Wrap(err, "head")
		}
	}
	if head < synced {
		return head, errors.Errorf("head is step %d, but step %d was synced", head, synced)
	}

	// Every step through the head has its reference, and none after it.
	for i := 0; i <= head; i++ {
		v, found, err := tx.Get(ctx, refKey(i))
		if err != nil {
			return head, err
		}
		if !found {
			return head, errors.Errorf("step %d of %d has no reference", i, head)
		}
		ref, err := block.UnmarshalBlockRefB58(string(v))
		if err != nil {
			return head, errors.Wrapf(err, "step %d reference", i)
		}
		data, found, err := s.GetBlock(ctx, ref)
		if err != nil {
			return head, errors.Wrapf(err, "step %d block", i)
		}
		if !found {
			if i <= synced {
				return head, errors.Errorf("synced step %d lost its block", i)
			}
			continue
		}
		if !bytes.Equal(data, payload(i)) {
			return head, errors.Errorf("step %d block is corrupt", i)
		}
	}
	if _, found, err := tx.Get(ctx, refKey(head+1)); err != nil || found {
		return head, errors.Errorf("step %d follows head %d (err %v)", head+1, head, err)
	}
	return head, nil
}

// refKey returns the key of step i's block reference.
func refKey(i int) []byte {
	return []byte(fmt.Sprintf("ref/%08d", i))
}

// payload returns step i's block: the step number, then a pattern it seeds.
func payload(i int) []byte {
	data := make([]byte, stepBlockSize)
	binary.BigEndian.PutUint64(data, uint64(i)) //nolint:gosec
	for j := 8; j < len(data); j++ {
		data[j] = byte(i*131 + j)
	}
	return data
}

// channel receives the messages posted on the reliability BroadcastChannel.
type channel struct {
	// bc is the BroadcastChannel.
	bc js.Value
	// onMessage forwards each message to msgs.
	onMessage js.Func
	// msgs holds the received messages.
	msgs chan string
}

// listen starts receiving on the reliability channel.
func listen() *channel {
	c := &channel{
		bc:   js.Global().Get("BroadcastChannel").New(reliabilityChannel),
		msgs: make(chan string, 16),
	}
	c.onMessage = js.FuncOf(func(this js.Value, args []js.Value) any {
		c.msgs <- args[0].Get("data").String()
		return nil
	})
	c.bc.Set("onmessage", c.onMessage)
	return c
}

// wait returns once msg arrives, discarding other messages.
func (c *channel) wait(ctx context.Context, msg string) error {
	logf("waiting for %s", msg)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case got := <-c.msgs:
			if got == msg {
				return nil
			}
		}
	}
}

// post sends msg to every other listener.
func (c *channel) post(msg string) {
	c.bc.Call("postMessage", msg)
}

// close stops receiving.
func (c *channel) close() {
	c.bc.Call("close")
	c.onMessage.Release()
}

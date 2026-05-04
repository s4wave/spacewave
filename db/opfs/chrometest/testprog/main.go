//go:build js

package main

import (
	"context"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/metashard"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

type config struct {
	scenario   string
	root       string
	worker     int
	workers    int
	iterations int
	batch      int
	shards     int
}

type blockEvent struct {
	typ       string
	worker    int
	iteration int
}

type blockEventSub struct {
	ch chan blockEvent
	bc js.Value
	cb js.Func
}

type blockEventPub struct {
	bc js.Value
}

func main() {
	start := time.Now()
	c, err := parseConfig(os.Args)
	if err == nil {
		err = run(context.Background(), c)
	}
	postResult(c, time.Since(start), err)
}

func parseConfig(args []string) (*config, error) {
	if len(args) < 8 {
		return nil, errors.Errorf("expected 7 args, got %d", len(args)-1)
	}
	worker, err := strconv.Atoi(args[3])
	if err != nil {
		return nil, errors.Wrap(err, "parse worker")
	}
	workers, err := strconv.Atoi(args[4])
	if err != nil {
		return nil, errors.Wrap(err, "parse workers")
	}
	iterations, err := strconv.Atoi(args[5])
	if err != nil {
		return nil, errors.Wrap(err, "parse iterations")
	}
	batch, err := strconv.Atoi(args[6])
	if err != nil {
		return nil, errors.Wrap(err, "parse batch")
	}
	shards, err := strconv.Atoi(args[7])
	if err != nil {
		return nil, errors.Wrap(err, "parse shards")
	}
	return &config{
		scenario:   args[1],
		root:       args[2],
		worker:     worker,
		workers:    workers,
		iterations: iterations,
		batch:      batch,
		shards:     shards,
	}, nil
}

func run(ctx context.Context, c *config) error {
	switch c.scenario {
	case "clear":
		return clearRoot(c.root)
	case "block-writer":
		return runBlockWriter(ctx, c)
	case "block-reader":
		return runBlockReader(ctx, c)
	case "block-verify":
		return runBlockVerify(ctx, c)
	case "meta-writer":
		return runMetaWriter(ctx, c)
	case "meta-verify":
		return runMetaVerify(ctx, c)
	case "counter-init":
		return runCounterInit(c)
	case "counter-increment":
		return runCounterIncrement(c)
	case "counter-verify":
		return runCounterVerify(c)
	default:
		return errors.Errorf("unknown scenario %q", c.scenario)
	}
}

func clearRoot(rootName string) error {
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	err = opfs.DeleteEntry(root, rootName, true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	_, err = opfs.GetDirectory(root, rootName, true)
	return err
}

func runBlockWriter(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventPub(c.root)
	defer events.Close()
	defer events.Post(blockEvent{
		typ:    "block-writer-done",
		worker: c.worker,
	})

	for i := 0; i < c.iterations; i++ {
		entries := make([]segment.Entry, c.batch)
		for j := range entries {
			key := blockKey(c.worker, i, j)
			entries[j] = segment.Entry{
				Key:   key,
				Value: blockValue(key),
			}
		}
		if err := e.Put(ctx, entries); err != nil {
			return errors.Wrap(err, "put block batch")
		}
		events.Post(blockEvent{
			typ:       "block-written",
			worker:    c.worker,
			iteration: i,
		})
		if i%4 == 0 {
			key := blockKey(c.worker, i, 0)
			val, found, err := e.GetContext(ctx, key)
			if err != nil {
				return errors.Wrap(err, "read own block")
			}
			if !found || string(val) != string(blockValue(key)) {
				return errors.Errorf("own block mismatch worker=%d iteration=%d found=%v", c.worker, i, found)
			}
		}
	}
	return nil
}

func runBlockReader(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventSub(c)
	defer events.Close()
	postReady(c)

	done := make([]bool, c.workers)
	var found int
	var doneCount int
	for doneCount < c.workers {
		ev, err := events.Next(ctx)
		if err != nil {
			return err
		}
		switch ev.typ {
		case "block-written":
			for j := 0; j < c.batch; j++ {
				key := blockKey(ev.worker, ev.iteration, j)
				val, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read concurrent block")
				}
				if !ok {
					continue
				}
				if string(val) != string(blockValue(key)) {
					return errors.Errorf("block value mismatch key=%s", string(key))
				}
				found++
			}
		case "block-writer-done":
			if ev.worker < 0 || ev.worker >= len(done) {
				return errors.Errorf("invalid writer id %d", ev.worker)
			}
			if !done[ev.worker] {
				done[ev.worker] = true
				doneCount++
			}
		default:
			continue
		}
	}
	if found == 0 {
		for w := 0; w < c.workers; w++ {
			for i := 0; i < c.iterations; i++ {
				key := blockKey(w, i, 0)
				val, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read final concurrent block")
				}
				if ok {
					found++
					if string(val) != string(blockValue(key)) {
						return errors.Errorf("block value mismatch key=%s", string(key))
					}
				}
			}
		}
	}
	if found > 0 {
		return nil
	}
	return errors.New("reader found no concurrently written blocks")
}

func runBlockVerify(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()

	for w := 0; w < c.workers; w++ {
		for i := 0; i < c.iterations; i++ {
			for j := 0; j < c.batch; j++ {
				key := blockKey(w, i, j)
				val, found, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "verify block")
				}
				if !found {
					return errors.Errorf("missing block key=%s %s", string(key), describeBlockShard(c, e.ShardForKey(key)))
				}
				if string(val) != string(blockValue(key)) {
					return errors.Errorf("bad block value key=%s", string(key))
				}
			}
		}
	}
	return nil
}

func openBlockEngine(ctx context.Context, c *config) (*blockshard.Engine, func(), error) {
	dir, err := openTestDirectory(c.root, []string{"blocks"})
	if err != nil {
		return nil, nil, err
	}
	settings := blockshard.DefaultSettings()
	settings.ShardCount = c.shards
	settings.AsyncIO = true
	e, err := blockshard.NewEngineWithSettings(ctx, dir, c.root+"/blocks", settings)
	if err != nil {
		return nil, nil, err
	}
	return e, e.Close, nil
}

func describeBlockShard(c *config, shard int) string {
	dir, err := openTestDirectory(c.root, []string{"blocks", "shard-" + zeroPad(shard, 2)})
	if err != nil {
		return "describe-shard-error=" + err.Error()
	}
	a, err := opfs.ReadFile(dir, "manifest-a")
	if err != nil && !opfs.IsNotFound(err) {
		return "read-manifest-a-error=" + err.Error()
	}
	b, err := opfs.ReadFile(dir, "manifest-b")
	if err != nil && !opfs.IsNotFound(err) {
		return "read-manifest-b-error=" + err.Error()
	}
	m := blockshard.PickManifest(a, b)
	if m == nil {
		return "manifest=nil"
	}
	var sb strings.Builder
	sb.WriteString("shard=")
	sb.WriteString(strconv.Itoa(shard))
	sb.WriteString(" gen=")
	sb.WriteString(strconv.FormatUint(m.Generation, 10))
	sb.WriteString(" segments=")
	sb.WriteString(strconv.Itoa(len(m.Segments)))
	limit := len(m.Segments)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		seg := m.Segments[i]
		sb.WriteString(" ")
		sb.WriteString(seg.Filename)
		sb.WriteString("[")
		sb.Write(seg.MinKey)
		sb.WriteString("..")
		sb.Write(seg.MaxKey)
		sb.WriteString("]")
	}
	return sb.String()
}

func runMetaWriter(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return errors.Wrap(err, "open meta write tx")
		}
		key := metaKey(c.worker, i)
		if err := tx.Set(ctx, key, metaValue(key)); err != nil {
			tx.Discard()
			return errors.Wrap(err, "set meta")
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit meta")
		}
		if i%5 == 0 {
			if err := verifyMetaKey(ctx, store, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func runMetaVerify(ctx context.Context, c *config) error {
	store, err := openMetaStore(c)
	if err != nil {
		return err
	}
	for w := 0; w < c.workers; w++ {
		for i := 0; i < c.iterations; i++ {
			if err := verifyMetaKey(ctx, store, metaKey(w, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func openMetaStore(c *config) (*metashard.MetaStore, error) {
	dir, err := openTestDirectory(c.root, []string{"meta"})
	if err != nil {
		return nil, err
	}
	shard, err := metashard.NewMetaShard(dir, c.root+"/meta", 4096)
	if err != nil {
		return nil, err
	}
	return metashard.NewMetaStore(shard), nil
}

func verifyMetaKey(ctx context.Context, store *metashard.MetaStore, key []byte) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open meta read tx")
	}
	defer tx.Discard()
	val, found, err := tx.Get(ctx, key)
	if err != nil {
		return errors.Wrap(err, "get meta")
	}
	if !found {
		return errors.Errorf("missing meta key=%s", string(key))
	}
	if string(val) != string(metaValue(key)) {
		return errors.Errorf("bad meta value key=%s", string(key))
	}
	return nil
}

func runCounterInit(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
	if err != nil {
		return err
	}
	defer release()
	var zero [8]byte
	if err := file.Truncate(int64(len(zero))); err != nil {
		return err
	}
	if _, err := file.WriteAt(zero[:], 0); err != nil {
		return err
	}
	return file.Flush()
}

func runCounterIncrement(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	for i := 0; i < c.iterations; i++ {
		file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", true)
		if err != nil {
			return errors.Wrap(err, "acquire counter")
		}
		var buf [8]byte
		if _, err := file.ReadAt(buf[:], 0); err != nil {
			release()
			return errors.Wrap(err, "read counter")
		}
		val := binary.LittleEndian.Uint64(buf[:])
		binary.LittleEndian.PutUint64(buf[:], val+1)
		if _, err := file.WriteAt(buf[:], 0); err != nil {
			release()
			return errors.Wrap(err, "write counter")
		}
		if err := file.Flush(); err != nil {
			release()
			return errors.Wrap(err, "flush counter")
		}
		release()
	}
	return nil
}

func runCounterVerify(c *config) error {
	dir, err := openTestDirectory(c.root, []string{"locks"})
	if err != nil {
		return err
	}
	file, release, err := filelock.AcquireFile(dir, "counter", c.root+"/locks", false)
	if err != nil {
		return err
	}
	defer release()
	var buf [8]byte
	if _, err := file.ReadAt(buf[:], 0); err != nil {
		return err
	}
	got := binary.LittleEndian.Uint64(buf[:])
	want := uint64(c.workers * c.iterations)
	if got != want {
		return errors.Errorf("counter=%d want=%d", got, want)
	}
	return nil
}

func openTestDirectory(rootName string, parts []string) (js.Value, error) {
	root, err := opfs.GetRoot()
	if err != nil {
		return js.Undefined(), err
	}
	path := append([]string{rootName}, parts...)
	return opfs.GetDirectoryPath(root, path, true)
}

func blockKey(worker, iteration, entry int) []byte {
	return []byte("b/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5) + "/" + zeroPad(entry, 3))
}

func blockValue(key []byte) []byte {
	return []byte("value:" + string(key))
}

func metaKey(worker, iteration int) []byte {
	return []byte("m/" + strconv.Itoa(worker) + "/" + zeroPad(iteration, 5))
}

func metaValue(key []byte) []byte {
	return []byte("value:" + string(key))
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func newBlockEventSub(c *config) *blockEventSub {
	ch := make(chan blockEvent, c.workers*c.iterations+c.workers+8)
	bc := js.Global().Get("BroadcastChannel").New(blockEventChannel(c.root))
	cb := js.FuncOf(func(this js.Value, args []js.Value) any {
		data := args[0].Get("data")
		ch <- blockEvent{
			typ:       data.Get("type").String(),
			worker:    data.Get("worker").Int(),
			iteration: data.Get("iteration").Int(),
		}
		return nil
	})
	bc.Set("onmessage", cb)
	return &blockEventSub{
		ch: ch,
		bc: bc,
		cb: cb,
	}
}

func (s *blockEventSub) Next(ctx context.Context) (blockEvent, error) {
	select {
	case ev := <-s.ch:
		return ev, nil
	case <-ctx.Done():
		return blockEvent{}, ctx.Err()
	}
}

func (s *blockEventSub) Close() {
	s.bc.Set("onmessage", js.Null())
	s.bc.Call("close")
	s.cb.Release()
}

func newBlockEventPub(root string) *blockEventPub {
	return &blockEventPub{
		bc: js.Global().Get("BroadcastChannel").New(blockEventChannel(root)),
	}
}

func (p *blockEventPub) Post(ev blockEvent) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", ev.typ)
	obj.Set("worker", ev.worker)
	obj.Set("iteration", ev.iteration)
	p.bc.Call("postMessage", obj)
}

func (p *blockEventPub) Close() {
	p.bc.Call("close")
}

func blockEventChannel(root string) string {
	return "opfs-chrometest:" + root
}

func postReady(c *config) {
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "ready")
	obj.Set("scenario", c.scenario)
	obj.Set("worker", c.worker)
	js.Global().Call("postMessage", obj)
}

func postResult(c *config, dur time.Duration, err error) {
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "result")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
	}
	obj.Set("durationMs", dur.Milliseconds())
	if err != nil {
		obj.Set("ok", false)
		obj.Set("error", err.Error())
	} else {
		obj.Set("ok", true)
	}
	js.Global().Call("postMessage", obj)
}

//go:build js

package main

import (
	"context"
	"os"
	"strconv"
	"syscall/js"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
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
	// Parse one worker scenario from the browser launch arguments.
	started := time.Now()
	c, err := parseConfig(testArgs())

	// Run the selected storage action.
	if err == nil {
		err = run(context.Background(), c)
	}

	// Return one result before the WebAssembly runtime exits.
	postResult(c, time.Since(started), err)
}

func testArgs() []string {
	if len(os.Args) >= 8 {
		return os.Args
	}
	value := js.Global().Get("__OPFS_CHROMETEST_ARGS")
	if value.IsUndefined() || value.IsNull() {
		return os.Args
	}
	args := make([]string, value.Get("length").Int())
	for i := range args {
		args[i] = value.Index(i).String()
	}
	return args
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
	opfs.InstallRemoteDriverFromGlobal()
	switch c.scenario {
	case "clear":
		return clearRoot(c.root)
	case "block-writer":
		return runBlockWriter(ctx, c)
	case "block-reader":
		return runBlockReader(ctx, c, false)
	case "block-reader-compact":
		return runBlockReader(ctx, c, true)
	case "block-verify":
		return runBlockVerify(ctx, c)
	default:
		return errors.Errorf("unknown cache probe scenario %q", c.scenario)
	}
}

func clearRoot(rootName string) error {
	// Open the browser OPFS root.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}

	// Reset and recreate the named shared-volume directory.
	err = opfs.DeleteEntry(root, rootName, true)
	if err != nil && !opfs.IsNotFound(err) {
		return err
	}
	_, err = opfs.GetDirectory(root, rootName, true)
	return err
}

func runBlockWriter(ctx context.Context, c *config) error {
	// Open one publishing engine and its cross-runtime event channel.
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventPub(c.root)
	defer events.Close()

	// Publish deterministic batches and announce each visible generation.
	for i := range c.iterations {
		entries := make([]segment.Entry, c.batch)
		for j := range entries {
			key := blockKey(c.worker, i, j)
			entries[j] = segment.Entry{Key: key, Value: blockValue(key)}
		}
		if err := e.Put(ctx, entries); err != nil {
			return errors.Wrap(err, "write concurrent blocks")
		}
		events.Post(blockEvent{typ: "block-written", worker: c.worker, iteration: i})
	}

	// Announce completion after every published batch is durable.
	events.Post(blockEvent{typ: "block-writer-done", worker: c.worker})
	return nil
}

func runBlockReader(ctx context.Context, c *config, compact bool) error {
	// Open one reader before the publishers start.
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	events := newBlockEventSub(c)
	defer events.Close()
	postReady(c)

	// Consume publication events until every writer completes.
	done := make([]bool, c.workers)
	var found int
	var doneCount int
	for doneCount < c.workers {
		event, err := events.Next(ctx)
		if err != nil {
			return err
		}
		switch event.typ {
		case "block-written":
			for j := range c.batch {
				key := blockKey(event.worker, event.iteration, j)
				value, ok, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read concurrent block")
				}
				if !ok {
					continue
				}
				if string(value) != string(blockValue(key)) {
					return errors.Errorf("block value mismatch key=%s", string(key))
				}
				found++
			}
		case "block-writer-done":
			if event.worker < 0 || event.worker >= len(done) {
				return errors.Errorf("invalid writer id %d", event.worker)
			}
			if !done[event.worker] {
				done[event.worker] = true
				doneCount++
			}
		}
	}
	if found == 0 {
		return errors.New("reader found no concurrently written blocks")
	}
	if !compact {
		return nil
	}

	// Compact through the live reader before checking all values.
	if err := e.CompactOnce(ctx); err != nil {
		return errors.Wrap(err, "compact shared block volume")
	}
	return verifyBlocks(ctx, c, e, "after compaction")
}

func runBlockVerify(ctx context.Context, c *config) error {
	e, release, err := openBlockEngine(ctx, c)
	if err != nil {
		return err
	}
	defer release()
	return verifyBlocks(ctx, c, e, "after remount")
}

func verifyBlocks(ctx context.Context, c *config, e *blockshard.Engine, phase string) error {
	for w := range c.workers {
		for i := range c.iterations {
			for j := range c.batch {
				key := blockKey(w, i, j)
				value, found, err := e.GetContext(ctx, key)
				if err != nil {
					return errors.Wrap(err, "read block "+phase)
				}
				if !found {
					return errors.Errorf("missing block %s key=%s", phase, string(key))
				}
				if string(value) != string(blockValue(key)) {
					return errors.Errorf("bad block %s key=%s", phase, string(key))
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
	e, err := blockshard.NewEngineWithSettings(ctx, dir, c.root+"/blocks", settings)
	if err != nil {
		return nil, nil, err
	}
	return e, e.Close, nil
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

func zeroPad(n, width int) string {
	value := strconv.Itoa(n)
	for len(value) < width {
		value = "0" + value
	}
	return value
}

func newBlockEventSub(c *config) *blockEventSub {
	ch := make(chan blockEvent, c.workers*c.iterations+c.workers+8)
	bc := js.Global().Get("BroadcastChannel").New(blockEventChannel(c.root))
	cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		ch <- blockEvent{
			typ:       data.Get("type").String(),
			worker:    data.Get("worker").Int(),
			iteration: data.Get("iteration").Int(),
		}
		return nil
	})
	bc.Set("onmessage", cb)
	return &blockEventSub{ch: ch, bc: bc, cb: cb}
}

func (s *blockEventSub) Next(ctx context.Context) (blockEvent, error) {
	select {
	case event := <-s.ch:
		return event, nil
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
	return &blockEventPub{bc: js.Global().Get("BroadcastChannel").New(blockEventChannel(root))}
}

func (p *blockEventPub) Post(event blockEvent) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", event.typ)
	obj.Set("worker", event.worker)
	obj.Set("iteration", event.iteration)
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

func postResult(c *config, duration time.Duration, err error) {
	// Build the shared worker result fields.
	obj := js.Global().Get("Object").New()
	obj.Set("kind", "result")
	if c != nil {
		obj.Set("scenario", c.scenario)
		obj.Set("worker", c.worker)
	}

	// Attach the terminal status.
	obj.Set("durationMs", duration.Milliseconds())
	obj.Set("ok", true)
	if err != nil {
		obj.Set("ok", false)
		obj.Set("error", err.Error())
	}

	// Publish the terminal result to the browser harness.
	js.Global().Call("postMessage", obj)
}

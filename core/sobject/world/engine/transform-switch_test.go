package sobject_world_engine_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
)

// gzipMagic starts every gzip stream.
var gzipMagic = []byte{0x1f, 0x8b, 0x08}

// TestSetTransformConfSwitchesLiveWorld checks that a Space World written with
// gzip switches to s2 through its engine: later commits store s2 blocks, the
// World still reads the gzip blocks, and an invalid successor is refused. The
// engine stores every World so each commit's blocks reach the volume.
func TestSetTransformConfSwitchesLiveWorld(t *testing.T) {
	// Start a World on gzip and write an object.
	ctx := t.Context()
	enc := &transform_blockenc.Config{BlockEnc: blockenc.DefaultBlockEnc, Key: bytes.Repeat([]byte{3}, 32)}
	gzipConf := newSwitchTestConf(t, &transform_gzip.Config{}, enc)
	sw := newSpaceWorldWithConf(ctx, t, func(conf *sobject_world_engine.Config) {
		conf.InitWorldOp.TransformConf = gzipConf
		conf.StoreEveryWorld = true
	})
	commitSwitchTestObject(ctx, t, sw.eng, "before")
	before := sw.worldBlocks(ctx, t, enc)
	if len(before) == 0 || !allGzip(before) {
		t.Fatalf("gzip World holds %d blocks, all gzip %v", len(before), allGzip(before))
	}

	// Switch to s2, refusing a successor that cannot read gzip.
	s2Conf := newSwitchTestConf(t, &transform_s2.Config{ReadGzip: true}, enc)
	err := world.AccessEngine(ctx, sw.eng, func(ctx context.Context, eng world.Engine) error {
		// Refuse s2 without read_gzip, then accept it with read_gzip.
		te := eng.(sobject_world_engine.TransformEngine)
		if err := te.SetTransformConf(ctx, newSwitchTestConf(t, &transform_s2.Config{}, enc)); err == nil {
			t.Error("s2 without read_gzip accepted")
		}
		if err := te.SetTransformConf(ctx, s2Conf); err != nil {
			return err
		}

		// The head names the new transform.
		got, err := te.GetTransformConf(ctx)
		if err == nil && !got.EqualVT(s2Conf) {
			t.Error("head does not name the s2 transform")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// The next commit stores s2 blocks.
	commitSwitchTestObject(ctx, t, sw.eng, "after")
	var added [][]byte
	for iri, data := range sw.worldBlocks(ctx, t, enc) {
		if _, ok := before[iri]; !ok {
			added = append(added, data)
		}
	}
	if len(added) == 0 {
		t.Fatal("commit after the switch stored no World blocks")
	}
	for _, data := range added {
		if bytes.HasPrefix(data, gzipMagic) {
			t.Fatal("commit after the switch stored a gzip block")
		}
	}

	// The World reads both objects.
	read, err := sw.eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	for _, key := range []string{"before", "after"} {
		obj, found, err := read.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("object %q found = %v, %v", key, found, err)
		}
	}
}

// worldBlocks returns the blocks reachable from the bucket that decrypt
// under enc, decrypted, by IRI.
func (w *spaceWorld) worldBlocks(ctx context.Context, t *testing.T, enc *transform_blockenc.Config) map[string][]byte {
	// Build the decryption-only transform.
	t.Helper()
	conf := newSwitchTestConf(t, enc)
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		t.Fatal(err)
	}

	// Walk the child lists from the engine's head, decrypting each block and
	// skipping blocks of another transform.
	lists := w.vol.(interface {
		GetChildren(context.Context, *block.BlockRef) ([]*block.BlockRef, bool, error)
	})
	out := make(map[string][]byte)
	head := engineHead(ctx, t, w.eng)
	seen := map[string]bool{head.MarshalString(): true}
	queue := []*block.BlockRef{head}
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		children, _, err := lists.GetChildren(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range children {
			if !seen[child.MarshalString()] {
				seen[child.MarshalString()] = true
				queue = append(queue, child)
			}
		}
		data, found, err := w.vol.GetBlock(ctx, ref)
		if err != nil || !found {
			continue
		}
		if dec, err := xfrm.DecodeBlock(data); err == nil {
			out[ref.MarshalString()] = dec
		}
	}
	return out
}

// allGzip reports whether every block is a gzip stream.
func allGzip(blocks map[string][]byte) bool {
	for _, data := range blocks {
		if !bytes.HasPrefix(data, gzipMagic) {
			return false
		}
	}
	return true
}

// newSwitchTestConf builds a transform config from steps.
func newSwitchTestConf(t *testing.T, steps ...config.Config) *block_transform.Config {
	t.Helper()
	conf, err := block_transform.NewConfig(steps)
	if err != nil {
		t.Fatal(err)
	}
	return conf
}

// commitSwitchTestObject creates the object key in one commit.
func commitSwitchTestObject(ctx context.Context, t *testing.T, eng world.Engine, key string) {
	// Open a write transaction.
	t.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Create the object and commit.
	obj, err := tx.CreateObject(ctx, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

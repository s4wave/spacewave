//go:build !js && !wasip1

package world_block_engine_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	sdk_cursor "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

type batchingFixture struct {
	tb     *world_testbed.Testbed
	client *resource_client.Client
	engine *sdk_world.Engine
	db     *bdb.DB
}

func newBatchingFixture(t testing.TB, history int) *batchingFixture {
	t.Helper()
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_bolt.Config{
		Path:         filepath.Join(t.TempDir(), "world.bolt"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	// Prepopulate an unrelated object store in one transaction outside the timer.
	store, rel, err := tb.Volume.AccessObjectStore(ctx, "unrelated-history", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	ktx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ktx.Discard()
	for i := 0; i < history; i++ {
		if err := ktx.Set(ctx, []byte(fmt.Sprintf("history/%08d", i)), make([]byte, 4096)); err != nil {
			t.Fatal(err)
		}
	}
	if err := ktx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err)
	}
	client, cleanup := resource_testbed.SetupResourceClient(ctx, t, wtb)
	t.Cleanup(cleanup)
	root := client.AccessRootResource()
	t.Cleanup(root.Release)
	srpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpc).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{EngineId: "batching-resource-world"})
	if err != nil {
		t.Fatal(err)
	}
	ref := client.CreateResourceReference(resp.ResourceId)
	eng, err := sdk_world.NewEngine(client, ref)
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	t.Cleanup(eng.Release)
	// Controller initialization creates the empty World lazily. Warm it before
	// measuring construction so setup writes do not masquerade as batch savings.
	if _, err := eng.GetSeqno(ctx); err != nil {
		t.Fatal(err)
	}
	db := volume_bolt.GetBoltDB(tb.Volume)
	if db == nil {
		t.Fatal("not a native Bolt volume")
	}
	if db.NoSync || db.NoFreelistSync {
		t.Fatal("durability must remain enabled")
	}
	return &batchingFixture{tb: wtb, client: client, engine: eng, db: db}
}

// batchingResourceUpdate times the entire public Resource operation, and records
// construction and Commit separately. Setup and exact readback are outside the
// complete-update timer. Physical counts come from bbolt, not logical wrappers.
func batchingResourceUpdate(t testing.TB, f *batchingFixture, sample, count int) (time.Duration, time.Duration, time.Duration, uint64, uint64, uint32) {
	t.Helper()
	ctx := t.Context()
	prefix := fmt.Sprintf("batch/%04d/", sample)
	before := f.db.CommitCounter()
	start := time.Now()
	wtx, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Release()
	defer wtx.Discard(context.Background()) // independently release the remote resource
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = fmt.Sprintf("%s%04d", prefix, i)
		id, err := wtx.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = sdk_cursor.AccessCursor(ctx, f.client, id, func(c *bucket_lookup.Cursor) error {
			btx, bcs := c.BuildTransaction(nil)
			bcs.SetBlock(block_mock.NewExample(fmt.Sprintf("complete payload %04d\n%s", i, keys[i])), true)
			root, _, err := btx.Write(ctx, true)
			if err != nil {
				return err
			}
			ref := c.GetRef().Clone()
			ref.RootRef = root
			obj, err := wtx.CreateObject(ctx, keys[i], ref)
			world.ReleaseObjectState(obj)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			if err := wtx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(keys[i-1], "next", keys[i], "")); err != nil {
				t.Fatal(err)
			}
		}
	}
	prepared := f.db.CommitCounter()
	commitStart := time.Now()
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	commitElapsed := time.Since(commitStart)
	totalElapsed := time.Since(start)
	after := f.db.CommitCounter()
	readStart := time.Now()
	rtx, err := f.engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Release()
	defer rtx.Discard(context.Background())
	h := fnv.New32a()
	for i, key := range keys {
		obj, found, err := rtx.GetObject(ctx, key)
		if err != nil || !found {
			world.ReleaseObjectState(obj)
			t.Fatalf("read %s: found=%v err=%v", key, found, err)
		}
		ref, _, err := obj.GetRootRef(ctx)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		id, err := rtx.AccessWorldState(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		err = sdk_cursor.AccessCursor(ctx, f.client, id, func(c *bucket_lookup.Cursor) error {
			_, bcs := c.BuildTransaction(nil)
			body, err := block_mock.UnmarshalExample(ctx, bcs)
			if err != nil {
				return err
			}
			want := fmt.Sprintf("complete payload %04d\n%s", i, key)
			if body.GetMsg() != want {
				return fmt.Errorf("body mismatch %s: %q", key, body.GetMsg())
			}
			fmt.Fprintln(h, want)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			qs, err := rtx.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(keys[i-1], "next", key, ""), 0)
			if err != nil || len(qs) != 1 {
				t.Fatalf("relationship %s: %d %v", key, len(qs), err)
			}
		}
	}
	return totalElapsed, commitElapsed, time.Since(readStart), prepared - before, after - prepared, h.Sum32()
}

func TestWorldCommitBatchingResourceBaseline(t *testing.T) {
	samples := 3
	if s := os.Getenv("SPACEWAVE_BATCHING_SAMPLES"); s != "" {
		var err error
		samples, err = strconv.Atoi(s)
		if err != nil || samples < 1 {
			t.Fatal("invalid sample count")
		}
	}
	for _, history := range []int{0, 512} {
		t.Run(fmt.Sprintf("history=%d", history), func(t *testing.T) {
			f := newBatchingFixture(t, history)
			for sample := 0; sample < samples; sample++ {
				total, commit, read, constructionCommits, publicationCommits, parity := batchingResourceUpdate(t, f, sample, 32)
				t.Logf("sample=%d objects=32 history=%d total_ns=%d commit_ns=%d readback_ns=%d construction_commits=%d publication_commits=%d parity=%d", sample, history, total, commit, read, constructionCommits, publicationCommits, parity)
			}
		})
	}
}

func BenchmarkWorldCommitBatchingResource(b *testing.B) {
	for _, n := range []int{1, 32, 128} {
		b.Run(fmt.Sprintf("objects=%d", n), func(b *testing.B) {
			f := newBatchingFixture(b, 512)
			b.ReportAllocs()
			b.ResetTimer()
			var total, commit, read time.Duration
			var construction, publication uint64
			for sample := 0; sample < b.N; sample++ {
				a, c, r, x, y, _ := batchingResourceUpdate(b, f, sample, n)
				total += a
				commit += c
				read += r
				construction += x
				publication += y
			}
			b.StopTimer()
			b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "complete-ns/op")
			b.ReportMetric(float64(commit.Nanoseconds())/float64(b.N), "commit-ns/op")
			b.ReportMetric(float64(read.Nanoseconds())/float64(b.N), "readback-ns/op")
			b.ReportMetric(float64(construction)/float64(b.N), "construction-commits/op")
			b.ReportMetric(float64(publication)/float64(b.N), "publication-commits/op")
		})
	}
}

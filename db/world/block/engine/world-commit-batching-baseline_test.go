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

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/s4db"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
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
	db     *s4db.DB
}

func newBatchingFixture(t testing.TB, history int) *batchingFixture {
	t.Helper()
	return newBatchingFixtureAt(t, history, filepath.Join(t.TempDir(), "world.s4wave"))
}

func newBatchingFixtureAt(t testing.TB, history int, path string) *batchingFixture {
	// Build the s4db storage testbed for the Resource batching fixture.
	t.Helper()
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := db_testbed.NewTestbed(ctx, logrus.NewEntry(log), db_testbed.WithVolumeConfig(&volume_s4db.Config{
		Path:         path,
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

	// Populate and commit the unrelated history before measurement.
	for i := range history {
		if err := ktx.Set(ctx, []byte(fmt.Sprintf("history/%08d", i)), make([]byte, 4096)); err != nil {
			t.Fatal(err)
		}
	}
	if err := ktx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Build the World testbed and access its root Resource client.
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

	// Create the remote World and retain its Resource engine.
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

	// Require the fixture to use a native s4db database.
	db := volume_s4db.GetDB(tb.Volume)
	if db == nil {
		t.Fatal("not a native s4db volume")
	}
	return &batchingFixture{tb: wtb, client: client, engine: eng, db: db}
}

// batchingResourceUpdate times the entire public Resource operation, and records
// construction and Commit separately. Setup and exact readback are outside the
// complete-update timer. Physical counts come from the s4db commit sequence,
// not logical wrappers; a record that starts a log chunk counts once more.
func batchingResourceUpdate(t testing.TB, f *batchingFixture, sample, count int) (time.Duration, time.Duration, time.Duration, uint64, uint64, uint32) {
	// Open the Resource World transaction and begin complete-update timing.
	t.Helper()
	ctx := t.Context()
	before := f.db.Seq()
	start := time.Now()
	wtx, err := f.engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Release()
	defer wtx.Discard(context.Background()) // independently release the remote resource

	// Prepare the Resource objects and record construction commits.
	keys := batchingResourcePopulate(t, ctx, f, wtx, sample, count)
	prepared := f.db.Seq()

	// Measure Resource publication latency and its physical commits.
	commitStart := time.Now()
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	commitElapsed := time.Since(commitStart)
	totalElapsed := time.Since(start)
	after := f.db.Seq()
	if b, ok := t.(*testing.B); ok {
		b.StopTimer()
	}

	// Read back Resource contents outside the measured update timer.
	readStart := time.Now()
	parity := batchingResourceReadback(t, ctx, f, keys)
	readElapsed := time.Since(readStart)
	if b, ok := t.(*testing.B); ok {
		b.StartTimer()
	}
	return totalElapsed, commitElapsed, readElapsed, prepared - before, after - prepared, parity
}

// batchingResourcePopulate uses only the public Resource cursor and World APIs.
func batchingResourcePopulate(t testing.TB, ctx context.Context, f *batchingFixture, wtx *sdk_world.Tx, sample, count int) []string {
	// Populate the Resource World with object bodies and graph relationships.
	t.Helper()
	prefix := fmt.Sprintf("batch/%04d/", sample)
	keys := make([]string, count)
	for i := range count {
		keys[i] = fmt.Sprintf("%s%04d", prefix, i)
		id, err := wtx.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = sdk_cursor.AccessCursor(ctx, f.client, id, func(c *bucket_lookup.Cursor) error {
			// Write the object payload through its Resource storage cursor.
			btx, bcs := c.BuildTransaction(nil)
			bcs.SetBlock(block_mock.NewExample(fmt.Sprintf("complete payload %04d\n%s", i, keys[i])), true)
			root, _, err := btx.Write(ctx, true)
			if err != nil {
				return err
			}

			// Create the World object with the written payload reference.
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
	return keys
}

func batchingResourceReadback(t testing.TB, ctx context.Context, f *batchingFixture, keys []string) uint32 {
	// Open a Resource World read snapshot for content parity.
	t.Helper()
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
			// Read the stored object payload through its Resource cursor.
			_, bcs := c.BuildTransaction(nil)
			body, err := block_mock.UnmarshalExample(ctx, bcs)
			if err != nil {
				return err
			}

			// Require the stored payload to match and contribute to the parity hash.
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
	return h.Sum32()
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
			// Prepare the Resource fixture and measurements for this object count.
			f := newBatchingFixture(b, 512)
			b.ReportAllocs()
			b.ResetTimer()
			var total, commit, read time.Duration
			var construction, publication uint64

			// Accumulate complete Resource update measurements for each sample.
			for sample := 0; sample < b.N; sample++ {
				a, c, r, x, y, _ := batchingResourceUpdate(b, f, sample, n)
				total += a
				commit += c
				read += r
				construction += x
				publication += y
			}

			// Report Resource update latency and physical commit counts.
			b.StopTimer()
			b.ReportMetric(float64(total.Nanoseconds())/float64(b.N), "complete-ns/op")
			b.ReportMetric(float64(commit.Nanoseconds())/float64(b.N), "commit-ns/op")
			b.ReportMetric(float64(read.Nanoseconds())/float64(b.N), "readback-ns/op")
			b.ReportMetric(float64(construction)/float64(b.N), "construction-commits/op")
			b.ReportMetric(float64(publication)/float64(b.N), "publication-commits/op")
		})
	}
}

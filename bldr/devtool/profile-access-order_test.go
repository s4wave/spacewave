//go:build !js

package devtool

import (
	"context"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

func TestRecordDynamicImportsRecordsEntrypointMatchesAndChunkDirectory(t *testing.T) {
	ctx := context.Background()
	distFS := newProfileAccessOrderTestFS(t, map[string][]byte{
		"entrypoint.mjs": []byte(`
import("./chunks/dot.mjs");
import("chunks/zeta.mjs");
import("chunks/zeta.mjs");
import("chunks/alpha.mjs");
import("chunks/not-js.css");
`),
		"chunks/alpha.mjs":                          []byte("export const alpha = true\n"),
		"chunks/beta.mjs":                           []byte("export const beta = true\n"),
		"chunks/ignored.js":                         []byte("export const ignored = true\n"),
		"chunks/zeta.mjs":                           []byte("export const zeta = true\n"),
		"entrypoint/abc123/runtime-goscript.mjs":    []byte(`runGoScriptRuntime(async () => (await import("./chunks/main.mjs")).main); import("chunks/worker-extra.mjs");`),
		"entrypoint/abc123/chunks/main.mjs":         []byte("export const main = true\n"),
		"entrypoint/abc123/chunks/worker-extra.mjs": []byte("export const workerExtra = true\n"),
	})
	defer distFS.Release()

	recorder := newStartupAccessRecorder()
	recorder.add(
		packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
		"./entrypoint.mjs",
		packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT,
		"startup",
	)
	if err := recordDynamicImports(ctx, recorder, distFS, "entrypoint.mjs"); err != nil {
		t.Fatalf("recordDynamicImports: %v", err)
	}

	entries := recorder.entries
	if len(entries) != 8 {
		t.Fatalf("got %d entries, want 8", len(entries))
	}
	assertProfileAccessEntry(t, entries[0], 0, "entrypoint.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT, "startup", 1)
	assertProfileAccessEntry(t, entries[1], 1, "chunks/dot.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "./chunks/dot.mjs", 1)
	assertProfileAccessEntry(t, entries[2], 2, "chunks/zeta.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "chunks/zeta.mjs", 3)
	assertProfileAccessEntry(t, entries[3], 3, "chunks/alpha.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "chunks/alpha.mjs", 2)
	assertProfileAccessEntry(t, entries[4], 4, "entrypoint/abc123/runtime-goscript.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT, "worker", 1)
	assertProfileAccessEntry(t, entries[5], 5, "entrypoint/abc123/chunks/main.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "./chunks/main.mjs", 2)
	assertProfileAccessEntry(t, entries[6], 6, "entrypoint/abc123/chunks/worker-extra.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "chunks/worker-extra.mjs", 2)
	assertProfileAccessEntry(t, entries[7], 7, "chunks/beta.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "chunks/beta.mjs", 1)
}

func TestRecordDynamicImportsResolvesRelativeChunkSpecifiers(t *testing.T) {
	ctx := context.Background()
	distFS := newProfileAccessOrderTestFS(t, map[string][]byte{
		"entrypoint/a/b/runtime.mjs": []byte(`
import("../../chunks/parent.mjs");
import("../chunks/sibling.mjs?worker#startup");
import("./chunks/local.mjs");
import("chunks/bare.mjs");
import("../../../chunks/outside.mjs");
import("/chunks/absolute.mjs");
import("../styles/not-a-chunk.mjs");
import("../chunks/not-module.js");
`),
	})
	defer distFS.Release()

	recorder := newStartupAccessRecorder()
	if err := recordDynamicImportsFromFile(ctx, recorder, distFS, "entrypoint/a/b/runtime.mjs"); err != nil {
		t.Fatalf("recordDynamicImportsFromFile: %v", err)
	}

	entries := recorder.entries
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5: %v", len(entries), entries)
	}
	assertProfileAccessEntry(t, entries[0], 0, "entrypoint/chunks/parent.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "../../chunks/parent.mjs", 1)
	assertProfileAccessEntry(t, entries[1], 1, "entrypoint/a/chunks/sibling.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "../chunks/sibling.mjs?worker#startup", 1)
	assertProfileAccessEntry(t, entries[2], 2, "entrypoint/a/b/chunks/local.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "./chunks/local.mjs", 1)
	assertProfileAccessEntry(t, entries[3], 3, "entrypoint/a/b/chunks/bare.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "chunks/bare.mjs", 1)
	assertProfileAccessEntry(t, entries[4], 4, "chunks/outside.mjs", packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_DYNAMIC_IMPORT, "../../../chunks/outside.mjs", 1)
}

func TestResolveStartupAccessRefsPopulatesResolvedRefsForRecordedDistAndAssetsEntries(t *testing.T) {
	ctx := context.Background()
	const sharedPath = "shared.txt"
	distBillyFS := newProfileAccessOrderTestBillyFS(t, map[string][]byte{
		sharedPath: []byte("dist startup module bytes\n"),
	})
	assetsBillyFS := newProfileAccessOrderTestBillyFS(t, map[string][]byte{
		sharedPath: []byte("asset startup bytes with a distinct block graph\n"),
	})

	bls := bucket_lookup.NewCursor(
		ctx,
		nil,
		nil,
		nil,
		bucket_mock.NewMockBucket("profile-access-order-resolved-refs", nil),
		block_transform.NewTransformerWithSteps(nil),
		nil,
		nil,
		nil,
	)
	meta := bldr_manifest.NewManifestMeta("resolved-ref-test", bldr_manifest.BuildType_DEV, "test/platform", 1)
	tx, bcs := bls.BuildTransaction(nil)
	manifest, err := bldr_manifest.CreateManifestWithBilly(
		ctx,
		bcs,
		meta,
		sharedPath,
		distBillyFS,
		assetsBillyFS,
		timestamppb.Now(),
	)
	if err != nil {
		t.Fatalf("CreateManifestWithBilly: %v", err)
	}
	if _, _, err := tx.Write(ctx, true); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	distEntry := &packfile_order.AccessOrderEntry{
		Filesystem: packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST,
		Path:       sharedPath,
		Reason:     packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ENTRYPOINT,
	}
	assetsEntry := &packfile_order.AccessOrderEntry{
		Filesystem: packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_ASSETS,
		Path:       sharedPath,
		Reason:     packfile_order.AccessOrderReason_ACCESS_ORDER_REASON_ASSET,
	}
	record := &packfile_order.AccessOrderRecord{
		Entries: []*packfile_order.AccessOrderEntry{distEntry, assetsEntry},
	}

	if err := resolveStartupAccessRefs(ctx, bls, manifest, record); err != nil {
		t.Fatalf("resolveStartupAccessRefs: %v", err)
	}

	distRefs := assertProfileAccessResolvedRefs(t, ctx, bls, distEntry)
	assetsRefs := assertProfileAccessResolvedRefs(t, ctx, bls, assetsEntry)
	if refsOverlap(distRefs, assetsRefs) {
		t.Fatalf("dist and assets entries for %q resolved to overlapping refs: dist=%v assets=%v", sharedPath, distRefs, assetsRefs)
	}
}

func assertProfileAccessResolvedRefs(
	t *testing.T,
	ctx context.Context,
	bls *bucket_lookup.Cursor,
	entry *packfile_order.AccessOrderEntry,
) map[string]struct{} {
	t.Helper()

	refs := entry.GetResolvedRefs()
	if len(refs) == 0 {
		t.Fatalf("%s %q resolved refs are empty", entry.GetFilesystem(), entry.GetPath())
	}
	keys := make(map[string]struct{}, len(refs))
	for idx, ref := range refs {
		if ref == nil || ref.GetEmpty() {
			t.Fatalf("%s %q resolved ref %d is empty", entry.GetFilesystem(), entry.GetPath(), idx)
		}
		key := ref.MarshalString()
		if _, ok := keys[key]; ok {
			t.Fatalf("%s %q resolved ref %d duplicates %s", entry.GetFilesystem(), entry.GetPath(), idx, key)
		}
		found, err := bls.GetBucket().GetBlockExists(ctx, ref)
		if err != nil {
			t.Fatalf("%s %q resolved ref %d lookup: %v", entry.GetFilesystem(), entry.GetPath(), idx, err)
		}
		if !found {
			t.Fatalf("%s %q resolved ref %d %s is not present in the manifest bucket", entry.GetFilesystem(), entry.GetPath(), idx, key)
		}
		keys[key] = struct{}{}
	}
	return keys
}

func refsOverlap(a, b map[string]struct{}) bool {
	for key := range a {
		if _, ok := b[key]; ok {
			return true
		}
	}
	return false
}

func assertProfileAccessEntry(
	t *testing.T,
	entry *packfile_order.AccessOrderEntry,
	ordinal uint64,
	fpath string,
	reason packfile_order.AccessOrderReason,
	detail string,
	accessCount uint64,
) {
	t.Helper()
	if entry.GetOrdinal() != ordinal {
		t.Fatalf("ordinal = %d, want %d", entry.GetOrdinal(), ordinal)
	}
	if entry.GetFilesystem() != packfile_order.AccessOrderFilesystem_ACCESS_ORDER_FILESYSTEM_DIST {
		t.Fatalf("filesystem = %s, want dist", entry.GetFilesystem())
	}
	if entry.GetPath() != fpath {
		t.Fatalf("path = %q, want %q", entry.GetPath(), fpath)
	}
	if entry.GetReason() != reason {
		t.Fatalf("reason = %s, want %s", entry.GetReason(), reason)
	}
	if entry.GetReasonDetail() != detail {
		t.Fatalf("detail = %q, want %q", entry.GetReasonDetail(), detail)
	}
	if entry.GetAccessCount() != accessCount {
		t.Fatalf("access count = %d, want %d", entry.GetAccessCount(), accessCount)
	}
}

func newProfileAccessOrderTestFS(t *testing.T, files map[string][]byte) *unixfs.FSHandle {
	t.Helper()

	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(newProfileAccessOrderTestBillyFS(t, files), ""))
	if err != nil {
		t.Fatal(err.Error())
	}
	return rootRef
}

func newProfileAccessOrderTestBillyFS(t *testing.T, files map[string][]byte) billy.Filesystem {
	t.Helper()

	fs := memfs.New()
	for fpath, body := range files {
		if err := billy_util.WriteFile(fs, fpath, body, 0o644); err != nil {
			t.Fatal(err.Error())
		}
	}
	return fs
}

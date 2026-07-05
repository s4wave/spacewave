//go:build !js

package devtool

import (
	"context"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
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

	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		t.Fatal(err.Error())
	}
	bfs := unixfs_billy.NewBillyFS(t.Context(), rootRef, "", time.Unix(0, 0))
	for fpath, body := range files {
		if err := billy_util.WriteFile(bfs, fpath, body, 0o644); err != nil {
			rootRef.Release()
			t.Fatal(err.Error())
		}
	}
	return rootRef
}

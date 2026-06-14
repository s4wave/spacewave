package unixfs_overlay

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_tar "github.com/s4wave/spacewave/db/unixfs/tar"
)

var overlayTestTime = time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

func TestOverlayFSCursor(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor)
	}{
		{
			name: "create over base",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				ops := mustOps(t, root)
				err := ops.MknodWithContent(
					ctx,
					"created.txt",
					unixfs.NewFSCursorNodeType_File(),
					int64(len("created")),
					bytes.NewReader([]byte("created")),
					0o644,
					overlayTestTime,
				)
				if err != nil {
					t.Fatal(err)
				}

				if got := mustReadFile(t, root, "created.txt"); got != "created" {
					t.Fatalf("created content: %q", got)
				}
				if _, err := mustOps(t, lower).Lookup(ctx, "created.txt"); err != unixfs_errors.ErrNotExist {
					t.Fatalf("lower lookup created.txt: %v", err)
				}
				if got := mustReadFile(t, upper, "created.txt"); got != "created" {
					t.Fatalf("upper content: %q", got)
				}
			},
		},
		{
			name: "modify base copy-up",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				child := mustLookup(t, root, "base.txt")
				childOps := mustOps(t, child)
				if got := mustReadFile(t, child, ""); got != "lower base" {
					t.Fatalf("pre-copy content: %q", got)
				}
				if err := childOps.Truncate(ctx, 0, overlayTestTime); err != nil {
					t.Fatal(err)
				}

				child = mustLookup(t, root, "base.txt")
				childOps = mustOps(t, child)
				if err := childOps.WriteAt(ctx, 0, []byte("upper base"), overlayTestTime); err != nil {
					t.Fatal(err)
				}

				if got := mustReadFile(t, root, "base.txt"); got != "upper base" {
					t.Fatalf("overlay content: %q", got)
				}
				if got := mustReadFile(t, lower, "base.txt"); got != "lower base" {
					t.Fatalf("lower content changed: %q", got)
				}
				if got := mustReadFile(t, upper, "base.txt"); got != "upper base" {
					t.Fatalf("upper content: %q", got)
				}
			},
		},
		{
			name: "delete base whiteout",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				if err := mustOps(t, root).Remove(ctx, []string{"base.txt"}, overlayTestTime); err != nil {
					t.Fatal(err)
				}
				if _, err := mustOps(t, root).Lookup(ctx, "base.txt"); err != unixfs_errors.ErrNotExist {
					t.Fatalf("overlay lookup removed base.txt: %v", err)
				}
				if got := mustReadFile(t, lower, "base.txt"); got != "lower base" {
					t.Fatalf("lower content changed: %q", got)
				}
				if !slices.Contains(mustReadDirNames(t, upper), ".wh.base.txt") {
					t.Fatalf("upper whiteout missing: %v", mustReadDirNames(t, upper))
				}
				if slices.Contains(mustReadDirNames(t, root), "base.txt") {
					t.Fatalf("removed base appears in overlay listing: %v", mustReadDirNames(t, root))
				}
			},
		},
		{
			name: "readdir union dedupe upper wins",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				upperOps := mustOps(t, upper)
				if err := upperOps.MknodWithContent(ctx, "base.txt", unixfs.NewFSCursorNodeType_File(), 5, bytes.NewReader([]byte("upper")), 0o644, overlayTestTime); err != nil {
					t.Fatal(err)
				}
				upperOps = mustOps(t, upper)
				if err := upperOps.MknodWithContent(ctx, "upper-only.txt", unixfs.NewFSCursorNodeType_File(), 10, bytes.NewReader([]byte("upper-only")), 0o644, overlayTestTime); err != nil {
					t.Fatal(err)
				}

				names := mustReadDirNames(t, root)
				want := []string{"base.txt", "dir", "lower-only.txt", "upper-only.txt"}
				if !slices.Equal(names, want) {
					t.Fatalf("names: got %v want %v", names, want)
				}
				if got := mustReadFile(t, root, "base.txt"); got != "upper" {
					t.Fatalf("upper did not win: %q", got)
				}
			},
		},
		{
			name: "nested mkdir parent-chain copy-up",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				dir := mustLookup(t, root, "dir")
				dirOps := mustOps(t, dir)
				if err := dirOps.Mknod(ctx, true, []string{"nested"}, unixfs.NewFSCursorNodeType_Dir(), 0o755, overlayTestTime); err != nil {
					t.Fatal(err)
				}

				if names := mustReadDirNames(t, upper); !slices.Contains(names, "dir") {
					t.Fatalf("upper parent dir was not copied up: %v", names)
				}
				upperDir := mustLookup(t, upper, "dir")
				if names := mustReadDirNames(t, upperDir); !slices.Contains(names, "nested") {
					t.Fatalf("nested dir missing in upper: %v", names)
				}
				if names := mustReadDirNames(t, lower); !slices.Contains(names, "dir") {
					t.Fatalf("lower dir missing unexpectedly: %v", names)
				}
			},
		},
		{
			name: "read pass-through from lower",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				if got := mustReadFile(t, root, "lower-only.txt"); got != "only lower" {
					t.Fatalf("pass-through content: %q", got)
				}
				if _, err := mustOps(t, upper).Lookup(context.Background(), "lower-only.txt"); err != unixfs_errors.ErrNotExist {
					t.Fatalf("upper lookup lower-only.txt: %v", err)
				}
			},
		},
		{
			name: "opaque dir hides lower entries",
			run: func(t *testing.T, root unixfs.FSCursor, lower unixfs.FSCursor, upper unixfs.FSCursor) {
				ctx := context.Background()
				upperOps := mustOps(t, upper)
				if err := upperOps.Mknod(ctx, true, []string{"dir"}, unixfs.NewFSCursorNodeType_Dir(), 0o755, overlayTestTime); err != nil {
					t.Fatal(err)
				}
				upperDir := mustLookup(t, upper, "dir")
				upperDirOps := mustOps(t, upperDir)
				if err := upperDirOps.MknodWithContent(ctx, ".wh..wh..opq", unixfs.NewFSCursorNodeType_File(), 0, bytes.NewReader(nil), 0, overlayTestTime); err != nil {
					t.Fatal(err)
				}
				upperDirOps = mustOps(t, upperDir)
				if err := upperDirOps.MknodWithContent(ctx, "upper-dir.txt", unixfs.NewFSCursorNodeType_File(), 9, bytes.NewReader([]byte("upper-dir")), 0o644, overlayTestTime); err != nil {
					t.Fatal(err)
				}

				overlayDir := mustLookup(t, root, "dir")
				names := mustReadDirNames(t, overlayDir)
				want := []string{"upper-dir.txt"}
				if !slices.Equal(names, want) {
					t.Fatalf("opaque names: got %v want %v", names, want)
				}
				if _, err := mustOps(t, overlayDir).Lookup(ctx, "lower-dir.txt"); err != unixfs_errors.ErrNotExist {
					t.Fatalf("opaque lookup lower-dir.txt: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lower := mustLower(t)
			upper := mustUpper(t)
			root := NewOverlayFSCursor(lower, upper)
			defer root.Release()
			tt.run(t, root, lower, upper)
		})
	}
}

func TestOverlayFSCursorReaddirAllCallbackCanReenterLookup(t *testing.T) {
	ctx := context.Background()
	lower := mustLower(t)
	upper := mustUpper(t)
	root := NewOverlayFSCursor(lower, upper)
	defer root.Release()

	ops := mustOps(t, root)
	var names []string
	err := ops.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		child, err := ops.Lookup(ctx, ent.GetName())
		if err != nil {
			return err
		}
		child.Release()
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"base.txt", "dir", "lower-only.txt"}
	if !slices.Equal(names, want) {
		t.Fatalf("names: got %v want %v", names, want)
	}
}

func mustLower(t *testing.T) unixfs.FSCursor {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mustWriteTarDir(t, tw, "dir/", 0o755)
	mustWriteTarFile(t, tw, "base.txt", "lower base", 0o644)
	mustWriteTarFile(t, tw, "lower-only.txt", "only lower", 0o644)
	mustWriteTarFile(t, tw, "dir/lower-dir.txt", "lower dir", 0o644)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	cursor, err := unixfs_tar.NewTarFSCursorFromReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return cursor
}

func mustUpper(t *testing.T) unixfs.FSCursor {
	t.Helper()

	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err)
	}
	return unixfs_billy.NewBillyFSCursor(bfs, "")
}

func mustOps(t *testing.T, cursor unixfs.FSCursor) unixfs.FSCursorOps {
	t.Helper()

	ops, err := cursor.GetCursorOps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func mustLookup(t *testing.T, cursor unixfs.FSCursor, name string) unixfs.FSCursor {
	t.Helper()

	child, err := mustOps(t, cursor).Lookup(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return child
}

func mustReadDirNames(t *testing.T, cursor unixfs.FSCursor) []string {
	t.Helper()

	var names []string
	err := mustOps(t, cursor).ReaddirAll(context.Background(), 0, func(ent unixfs.FSCursorDirent) error {
		names = append(names, ent.GetName())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	return names
}

func mustReadFile(t *testing.T, cursor unixfs.FSCursor, name string) string {
	t.Helper()

	fileCursor := cursor
	if name != "" {
		fileCursor = mustLookup(t, cursor, name)
	}
	ops := mustOps(t, fileCursor)
	size, err := ops.GetSize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, size)
	n, err := ops.ReadAt(context.Background(), 0, data)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return string(data[:n])
}

func mustWriteTarFile(t *testing.T, tw *tar.Writer, name string, content string, mode int64) {
	t.Helper()

	err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     mode,
		ModTime:  overlayTestTime,
		Typeflag: tar.TypeReg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}

func mustWriteTarDir(t *testing.T, tw *tar.Writer, name string, mode int64) {
	t.Helper()

	err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		ModTime:  overlayTestTime,
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		t.Fatal(err)
	}
}

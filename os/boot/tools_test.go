//go:build linux

package boot

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// checkImageTools inspects the public file interface on a reopened image.
func checkImageTools(t *testing.T, root *Root, expected []byte) {
	// Inspect fixed mountpoints and their exact modes through ordinary file metadata.
	t.Helper()
	ctx := t.Context()
	files := unixfs_iofs.NewFS(ctx, root.Handle())
	for _, name := range []string{"data", "dev", "proc", "tmp"} {
		info, err := files.Stat(name)
		if err != nil || info.Mode() != fs.ModeDir|0o755 {
			t.Fatalf("mountpoint %s: %v, %v", name, info, err)
		}
		entries, err := files.ReadDir(name)
		if err != nil || len(entries) != 0 {
			t.Fatalf("mountpoint %s not empty: %v", name, err)
		}
	}

	// Resolve every applet's relative target, including the shell, inside /bin.
	bin, err := root.Handle().Lookup(ctx, "bin")
	if err != nil {
		t.Fatal(err)
	}
	defer bin.Release()
	for applet := range strings.FieldsSeq(toolApplets) {
		entry, err := bin.Lookup(ctx, applet)
		if err != nil {
			t.Fatal(err)
		}
		target, absolute, err := entry.Readlink(ctx, "")
		entry.Release()
		if err != nil || absolute || !slices.Equal(target, []string{"busybox"}) {
			t.Fatalf("applet %s: %v, absolute=%v, %v", applet, target, absolute, err)
		}
		info, err := files.Stat("bin/" + applet)
		if err != nil || info.Mode() != fs.ModeSymlink|0o777 {
			t.Fatalf("applet mode %s: %v, %v", applet, info, err)
		}
		info, err = files.Stat("bin/" + strings.Join(target, "/"))
		if err != nil || info.Mode() != 0o755 || info.Size() != int64(len(expected)) {
			t.Fatalf("resolved applet %s: %v, %v", applet, info, err)
		}
	}

	// Read every byte beyond the import buffer, including a final data-plus-EOF read.
	if err := verifyImageFile(ctx, root.Handle(), "bin/busybox", bytes.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

// checkMissingTool removes an actually read tool descendant from a disposable copy.
func checkMissingTool(t *testing.T, original string, descendant *block.BlockRef, prefix []byte) {
	// Copy only this closed disposable database; keep the source adversaries separate.
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	path := filepath.Join(dir, "image.s4wave")
	src, err := os.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	if err := errors.Join(copyErr, dst.Close()); err != nil {
		t.Fatal(err)
	}

	// Preserve root/head/retention while removing one required tool content block.
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path, SingleProcess: true, NoGenerateKey: true, NoWriteKey: true})
	if err != nil {
		t.Fatal(err)
	}
	err = vol.Store.RmBlock(ctx, descendant)
	if err := errors.Join(err, vol.Close()); err != nil {
		t.Fatal(err)
	}

	// Runtime readiness must read the entire tool, not just its inode or first page.
	before := hashImageFile(t, path)
	root, err := OpenRoot(ctx, le, dir)
	if err != nil {
		t.Fatalf("tool adversary damaged the selected root: %v", err)
	}

	// A first-page probe still succeeds; only a complete tool read finds this loss.
	file, err := unixfs_iofs.NewFS(ctx, root.Handle()).Open("bin/busybox")
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, len(prefix))
	_, readErr := io.ReadFull(file, data)
	if err := errors.Join(readErr, file.Close()); err != nil || !bytes.Equal(data, prefix) {
		t.Fatalf("tool prefix should remain readable: %v", err)
	}

	// The normal readiness gate must reach and reject the missing later content.
	verifyErr := root.VerifyImage(ctx, bytes.NewBufferString("retained executable"))
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(verifyErr, block.ErrNotFound) || hashImageFile(t, path) != before {
		t.Fatalf("missing tool content refusal = %v; store must remain unchanged", verifyErr)
	}
	t.Log("INCOMPLETE_TOOL_REJECTED selected root retained; incomplete store preserved")
}

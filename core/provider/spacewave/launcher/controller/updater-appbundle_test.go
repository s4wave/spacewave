//go:build !js

package spacewave_launcher_controller

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarGzWithXattrs(t *testing.T) {
	// create a tar.gz with a fake .app structure
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// write .app directory structure
	entries := []struct {
		name    string
		content string
		mode    int64
		isDir   bool
		pax     map[string]string
	}{
		{name: "Test.app/", isDir: true, mode: 0o755},
		{name: "Test.app/Contents/", isDir: true, mode: 0o755},
		{name: "Test.app/Contents/MacOS/", isDir: true, mode: 0o755},
		{name: "Test.app/Contents/MacOS/spacewave", content: "#!/bin/bash\necho hello\n", mode: 0o755},
		{name: "Test.app/Contents/Info.plist", content: "<plist></plist>", mode: 0o644, pax: map[string]string{
			"SCHILY.xattr.com.apple.cs.CodeDirectory": "fakecdir",
		}},
		{name: "Test.app/Contents/Resources/", isDir: true, mode: 0o755},
		{name: "Test.app/Contents/Resources/icon.icns", content: "fakeicon", mode: 0o644},
	}

	for _, e := range entries {
		hdr := &tar.Header{
			Name:       e.name,
			Mode:       e.mode,
			PAXRecords: e.pax,
		}
		if e.isDir {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !e.isDir {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// extract
	destDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGzWithXattrs(archivePath, destDir); err != nil {
		t.Fatal(err)
	}

	// verify directory structure
	checks := []struct {
		path  string
		isDir bool
	}{
		{"Test.app", true},
		{"Test.app/Contents", true},
		{"Test.app/Contents/MacOS", true},
		{"Test.app/Contents/MacOS/spacewave", false},
		{"Test.app/Contents/Info.plist", false},
		{"Test.app/Contents/Resources", true},
		{"Test.app/Contents/Resources/icon.icns", false},
	}

	for _, chk := range checks {
		p := filepath.Join(destDir, chk.path)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("expected %s to exist: %v", chk.path, err)
			continue
		}
		if chk.isDir && !info.IsDir() {
			t.Errorf("expected %s to be a directory", chk.path)
		}
		if !chk.isDir && info.IsDir() {
			t.Errorf("expected %s to be a file", chk.path)
		}
	}

	// verify file content
	data, err := os.ReadFile(filepath.Join(destDir, "Test.app/Contents/MacOS/spacewave"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!/bin/bash\necho hello\n" {
		t.Errorf("unexpected binary content: %q", string(data))
	}

	// verify executable permission
	info, err := os.Stat(filepath.Join(destDir, "Test.app/Contents/MacOS/spacewave"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("expected binary to be executable")
	}
}

func TestExtractTarGzSymlinkBreakout(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "evil-symlink.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// In-tree symlink that escapes destDir via a relative target, followed by
	// a regular file written through it.
	entries := []*tar.Header{
		{Name: "Test.app/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "Test.app/escape", Typeflag: tar.TypeSymlink, Linkname: "../.."},
	}
	for _, hdr := range entries {
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte("pwned")
	fileHdr := &tar.Header{
		Name:     "Test.app/escape/evil",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(payload)),
	}
	if err := tw.WriteHeader(fileHdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err = extractTarGzWithXattrs(archivePath, destDir)
	if err == nil {
		t.Fatal("expected extraction to fail on symlink breakout")
	}

	// The file planted outside destDir must not exist.
	if _, err := os.Stat(filepath.Join(tmpDir, "evil")); err == nil {
		t.Error("symlink breakout attack succeeded: evil file created outside destDir")
	}
}

func TestExtractTarGzAbsoluteSymlinkRejected(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "abs-symlink.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	entries := []*tar.Header{
		{Name: "Test.app/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "Test.app/escape", Typeflag: tar.TypeSymlink, Linkname: "/etc"},
	}
	for _, hdr := range entries {
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := extractTarGzWithXattrs(archivePath, destDir); err == nil {
		t.Fatal("expected extraction to reject absolute symlink target")
	}
}

func TestExtractTarGzPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "evil.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// write a malicious entry with path traversal
	hdr := &tar.Header{
		Name:     "../../../etc/evil",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     4,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// should not create the traversal file
	if err := extractTarGzWithXattrs(archivePath, destDir); err != nil {
		t.Fatal(err)
	}

	// the evil file should not exist outside destDir
	if _, err := os.Stat(filepath.Join(tmpDir, "etc/evil")); err == nil {
		t.Error("path traversal attack succeeded: file created outside destDir")
	}
}

//go:build !js

package spacewave_launcher_controller

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRawEntrypointFromTar(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "spacewave.tar.gz")
	stagedPath := filepath.Join(dir, "spacewave")
	if err := writeTestTarGz(archivePath, rawEntrypointBinaryName(), []byte("bin")); err != nil {
		t.Fatal(err)
	}

	if err := extractRawEntrypointTarGz(archivePath, stagedPath); err != nil {
		t.Fatalf("extractRawEntrypointTarGz: %v", err)
	}
	got, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bin" {
		t.Fatalf("staged content = %q, want bin", string(got))
	}
}

func TestExtractRawEntrypointZip(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "spacewave.zip")
	stagedPath := filepath.Join(dir, "spacewave")
	if err := writeTestZip(archivePath, rawEntrypointBinaryName(), []byte("bin")); err != nil {
		t.Fatal(err)
	}

	if err := extractRawEntrypointZip(archivePath, stagedPath); err != nil {
		t.Fatalf("extractRawEntrypointZip: %v", err)
	}
	got, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bin" {
		t.Fatalf("staged content = %q, want bin", string(got))
	}
}

func TestExtractRawEntrypointMissing(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "spacewave.tar.gz")
	if err := writeTestTarGz(archivePath, "not-spacewave", []byte("bin")); err != nil {
		t.Fatal(err)
	}
	if err := extractRawEntrypointTarGz(archivePath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected missing raw entrypoint error")
	}
}

func writeTestTarGz(path, name string, body []byte) error {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(body)),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(body); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gzw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeTestZip(path, name string, body []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return zw.Close()
}

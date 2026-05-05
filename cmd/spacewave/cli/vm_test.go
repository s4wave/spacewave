//go:build !js

package spacewave_cli

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

func TestVmCommandShape(t *testing.T) {
	cmd := newVmCommand(nil)
	if cmd.Name != "vm" {
		t.Fatalf("name = %q, want vm", cmd.Name)
	}
	if len(cmd.Subcommands) != 7 {
		t.Fatalf("subcommand count = %d, want 7", len(cmd.Subcommands))
	}
	if cmd.Subcommands[0].Name != "list" {
		t.Fatalf("subcommand[0] = %q, want list", cmd.Subcommands[0].Name)
	}
	if cmd.Subcommands[1].Name != "info" {
		t.Fatalf("subcommand[1] = %q, want info", cmd.Subcommands[1].Name)
	}
	if cmd.Subcommands[2].Name != "create" {
		t.Fatalf("subcommand[2] = %q, want create", cmd.Subcommands[2].Name)
	}
	if cmd.Subcommands[3].Name != "start" {
		t.Fatalf("subcommand[3] = %q, want start", cmd.Subcommands[3].Name)
	}
	if cmd.Subcommands[4].Name != "stop" {
		t.Fatalf("subcommand[4] = %q, want stop", cmd.Subcommands[4].Name)
	}
	if cmd.Subcommands[5].Name != "watch" {
		t.Fatalf("subcommand[5] = %q, want watch", cmd.Subcommands[5].Name)
	}
	image := cmd.Subcommands[6]
	if image.Name != "image" {
		t.Fatalf("subcommand[6] = %q, want image", image.Name)
	}
	if len(image.Subcommands) != 1 {
		t.Fatalf("image subcommand count = %d, want 1", len(image.Subcommands))
	}
	if image.Subcommands[0].Name != "v86" {
		t.Fatalf("image subcommand[0] = %q, want v86", image.Subcommands[0].Name)
	}
	if len(image.Subcommands[0].Subcommands) != 4 {
		t.Fatalf("v86 subcommand count = %d, want 4", len(image.Subcommands[0].Subcommands))
	}
	if image.Subcommands[0].Subcommands[0].Name != "list" {
		t.Fatalf("v86 subcommand[0] = %q, want list", image.Subcommands[0].Subcommands[0].Name)
	}
	if image.Subcommands[0].Subcommands[1].Name != "info" {
		t.Fatalf("v86 subcommand[1] = %q, want info", image.Subcommands[0].Subcommands[1].Name)
	}
	if image.Subcommands[0].Subcommands[2].Name != "copy-from-cdn" {
		t.Fatalf("v86 subcommand[2] = %q, want copy-from-cdn", image.Subcommands[0].Subcommands[2].Name)
	}
	if image.Subcommands[0].Subcommands[3].Name != "import" {
		t.Fatalf("v86 subcommand[3] = %q, want import", image.Subcommands[0].Subcommands[3].Name)
	}
}

func TestParseV86MountFlags(t *testing.T) {
	mounts, err := parseV86MountFlags([]string{"/workspace=obj1:rw", "/home=obj2:ro", "/data=obj3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 3 {
		t.Fatalf("mount count = %d, want 3", len(mounts))
	}
	if !mounts[0].Writable || mounts[0].Path != "/workspace" || mounts[0].ObjectKey != "obj1" {
		t.Fatalf("bad rw mount: %+v", mounts[0])
	}
	if mounts[1].Writable || mounts[1].ObjectKey != "obj2" {
		t.Fatalf("bad ro mount: %+v", mounts[1])
	}
	if mounts[2].Writable || mounts[2].ObjectKey != "obj3" {
		t.Fatalf("bad default mount: %+v", mounts[2])
	}
	if _, err := parseV86MountFlags([]string{"/bad=obj:bad"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestValidateV86ImageImportTarArgs(t *testing.T) {
	dir := t.TempDir()
	args := &v86ImageImportTarArgs{
		wasmPath:      writeTestFile(t, dir, "v86.wasm", "wasm"),
		seabiosPath:   writeTestFile(t, dir, "seabios.bin", "seabios"),
		vgabiosPath:   writeTestFile(t, dir, "vgabios.bin", "vgabios"),
		kernelPath:    writeTestFile(t, dir, "bzImage", "kernel"),
		rootfsTarPath: writeTestTar(t, dir, "rootfs.tar"),
	}
	if err := validateV86ImageImportTarArgs(args); err != nil {
		t.Fatal(err)
	}

	args.kernelPath = filepath.Join(dir, "missing-bzImage")
	if err := validateV86ImageImportTarArgs(args); err == nil {
		t.Fatal("expected missing kernel error")
	}

	args.kernelPath = writeTestFile(t, dir, "bzImage2", "kernel")
	args.rootfsTarPath = writeTestFile(t, dir, "not-rootfs.tar", "not a tar")
	if err := validateV86ImageImportTarArgs(args); err == nil {
		t.Fatal("expected malformed tar error")
	}
}

func writeTestFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeTestTar(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	body := []byte("hello\n")
	if err := tw.WriteHeader(&tar.Header{Name: "etc/issue", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWriteV86ImageInfoJSON(t *testing.T) {
	entry := &v86ImageCLIEntry{
		objectKey: "v86image-test",
		image: &s4wave_vm.V86Image{
			Name:          "Debian",
			Version:       "1",
			Platform:      "v86",
			Distro:        "debian",
			KernelVersion: "6.1",
			Tags:          []string{"default"},
		},
		assets: map[string]string{
			"wasm":    "asset-wasm",
			"seabios": "asset-seabios",
			"vgabios": "asset-vgabios",
			"kernel":  "asset-kernel",
			"rootfs":  "asset-rootfs",
		},
	}
	var buf bytes.Buffer
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	err = writeV86ImageInfo(entry, "json")
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"\"objectKey\":\"v86image-test\"", "\"platform\":\"v86\"", "\"rootfs\":\"asset-rootfs\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %s: %s", want, out)
		}
	}
}

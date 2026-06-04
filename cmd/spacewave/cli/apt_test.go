//go:build !js

package spacewave_cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestAptCommandShape(t *testing.T) {
	cmd := newAptCommand(nil)
	if cmd.Name != "apt" {
		t.Fatalf("name = %q, want apt", cmd.Name)
	}
	if len(cmd.Subcommands) != 1 {
		t.Fatalf("subcommand count = %d, want 1", len(cmd.Subcommands))
	}
	importDeb := cmd.Subcommands[0]
	if importDeb.Name != "import-deb" {
		t.Fatalf("subcommand[0] = %q, want import-deb", importDeb.Name)
	}
	if importDeb.ArgsUsage != "<deb-path>" {
		t.Fatalf("import-deb ArgsUsage = %q, want <deb-path>", importDeb.ArgsUsage)
	}
}

func TestAptCommandRegistered(t *testing.T) {
	for _, cmd := range NewCliCommands(nil) {
		if cmd.Name == "apt" {
			return
		}
	}
	t.Fatal("apt command is not registered")
}

func TestValidateAptImportDebPath(t *testing.T) {
	dir := t.TempDir()
	debPath := filepath.Join(dir, "busybox_1.36.1-7_i386.deb")
	if err := os.WriteFile(debPath, []byte("deb"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := validateAptImportDebPath([]string{debPath})
	if err != nil {
		t.Fatal(err)
	}
	if got != debPath {
		t.Fatalf("validated path = %q, want %q", got, debPath)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: nil, want: "deb path required"},
		{name: "extra", args: []string{debPath, debPath}, want: "deb path required"},
		{name: "extension", args: []string{filepath.Join(dir, "busybox.txt")}, want: "must end in .deb"},
		{name: "missing-file", args: []string{filepath.Join(dir, "missing.deb")}, want: "stat deb path"},
		{name: "directory", args: []string{filepath.Join(dir, "pool.deb")}, want: "not a regular file"},
	}
	if err := os.Mkdir(filepath.Join(dir, "pool.deb"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		_, err := validateAptImportDebPath(c.args)
		if err == nil {
			t.Fatalf("%s: expected error", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: err = %q, want containing %q", c.name, err.Error(), c.want)
		}
	}
}

func TestAptImportDebActionParsesBeforeStorage(t *testing.T) {
	dir := t.TempDir()
	debPath := filepath.Join(dir, "busybox_1.36.1-7_i386.deb")
	if err := os.WriteFile(debPath, buildCliDebFixture(t), 0o644); err != nil {
		t.Fatal(err)
	}
	app := cli.NewApp()
	app.Name = "spacewave"
	flags := flag.NewFlagSet("import-deb", flag.ContinueOnError)
	if err := flags.Parse([]string{debPath}); err != nil {
		t.Fatal(err)
	}
	cmd := newAptImportDebCommand()
	if err := cmd.Action(cli.NewContext(app, flags, nil)); !errors.Is(err, errAptImportDebStoragePending) {
		t.Fatalf("import-deb err = %v, want pending storage", err)
	}
}

func TestAptImportDebActionRejectsInvalidDebBeforeStorage(t *testing.T) {
	dir := t.TempDir()
	debPath := filepath.Join(dir, "busybox_1.36.1-7_i386.deb")
	if err := os.WriteFile(debPath, []byte("deb"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := cli.NewApp()
	app.Name = "spacewave"
	flags := flag.NewFlagSet("import-deb", flag.ContinueOnError)
	if err := flags.Parse([]string{debPath}); err != nil {
		t.Fatal(err)
	}
	cmd := newAptImportDebCommand()
	if err := cmd.Action(cli.NewContext(app, flags, nil)); err == nil || errors.Is(err, errAptImportDebStoragePending) {
		t.Fatalf("import-deb err = %v, want parser error before storage", err)
	}
}

func buildCliDebFixture(t *testing.T) []byte {
	t.Helper()

	control := strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Description: Tiny utilities",
		"",
	}, "\n")
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "./control",
		Mode: 0o644,
		Size: int64(len(control)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(control)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var controlArchive bytes.Buffer
	gw := gzip.NewWriter(&controlArchive)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	writeCliArMember(t, &deb, "debian-binary", []byte("2.0\n"))
	writeCliArMember(t, &deb, "control.tar.gz", controlArchive.Bytes())
	writeCliArMember(t, &deb, "data.tar", nil)
	return deb.Bytes()
}

func writeCliArMember(t *testing.T, deb *bytes.Buffer, name string, data []byte) {
	t.Helper()

	header := cliArField(name+"/", 16) +
		cliArField("0", 12) +
		cliArField("0", 6) +
		cliArField("0", 6) +
		cliArField("100644", 8) +
		cliArField(strconv.Itoa(len(data)), 10) +
		"`\n"
	if len(header) != 60 {
		t.Fatalf("ar header size = %d, want 60", len(header))
	}
	deb.WriteString(header)
	if _, err := deb.Write(data); err != nil {
		t.Fatal(err)
	}
	if len(data)%2 != 0 {
		if err := deb.WriteByte('\n'); err != nil {
			t.Fatal(err)
		}
	}
}

func cliArField(value string, width int) string {
	return value + strings.Repeat(" ", width-len(value))
}

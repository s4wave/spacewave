package s4wave_apt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestParseDebPackageControlGzip(t *testing.T) {
	control := strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Depends: libc6 (>= 2.34), zlib1g",
		"Provides: busybox-static",
		"Conflicts: busybox-cvs",
		"Description: Tiny utilities for small systems",
		" BusyBox combines tiny versions of utilities.",
		" .",
		" second paragraph.",
		"",
	}, "\n")
	deb := buildDebFixture(t, "control.tar.gz", []byte(control))
	pkg, err := ParseDebPackage(deb)
	if err != nil {
		t.Fatalf("ParseDebPackage: %v", err)
	}
	if got := pkg.GetState(); got != AptPackageState_AptPackageState_IMPORTING {
		t.Fatalf("state = %s, want IMPORTING", got.String())
	}
	if pkg.GetName() != "busybox" {
		t.Fatalf("name = %q, want busybox", pkg.GetName())
	}
	if pkg.GetVersion() != "1:1.36.1-7" {
		t.Fatalf("version = %q, want 1:1.36.1-7", pkg.GetVersion())
	}
	if pkg.GetArchitecture() != "i386" {
		t.Fatalf("architecture = %q, want i386", pkg.GetArchitecture())
	}
	assertStringSlicesEqual(t, pkg.GetDepends(), []string{"libc6 (>= 2.34)", "zlib1g"})
	assertStringSlicesEqual(t, pkg.GetProvides(), []string{"busybox-static"})
	assertStringSlicesEqual(t, pkg.GetConflicts(), []string{"busybox-cvs"})
	wantDescription := "Tiny utilities for small systems\nBusyBox combines tiny versions of utilities.\n\nsecond paragraph."
	if pkg.GetDescription() != wantDescription {
		t.Fatalf("description = %q, want %q", pkg.GetDescription(), wantDescription)
	}
	if pkg.GetSize() != uint64(len(deb)) {
		t.Fatalf("size = %d, want %d", pkg.GetSize(), len(deb))
	}
	if err := pkg.Validate(); err != nil {
		t.Fatalf("parsed package should validate before deb_ref storage: %v", err)
	}
}

func TestParseDebPackageControlZstd(t *testing.T) {
	control := strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Description: Tiny utilities",
		"",
	}, "\n")
	deb := buildDebFixture(t, "control.tar.zst", []byte(control))
	pkg, err := ParseDebPackage(deb)
	if err != nil {
		t.Fatalf("ParseDebPackage(zstd): %v", err)
	}
	if pkg.GetName() != "busybox" {
		t.Fatalf("name = %q, want busybox", pkg.GetName())
	}
}

func TestParseDebPackageRejectsUnsupportedControlCompression(t *testing.T) {
	deb := buildDebArFixture(t, []debArTestMember{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: "control.tar.xz", data: []byte("xz-data")},
	})
	if _, err := ParseDebPackage(deb); !errors.Is(err, ErrUnsupportedDebControlCompression) {
		t.Fatalf("ParseDebPackage err = %v, want unsupported compression", err)
	}
}

func TestParseDebPackageRejectsInvalidDebianBinaryVersion(t *testing.T) {
	controlArchive := buildControlArchiveFixture(t, "control.tar.gz", []byte(strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Description: Tiny utilities",
		"",
	}, "\n")))
	deb := buildDebArFixture(t, []debArTestMember{
		{name: "debian-binary", data: []byte("1.0\n")},
		{name: "control.tar.gz", data: controlArchive},
		{name: "data.tar", data: nil},
	})
	if _, err := ParseDebPackage(deb); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ParseDebPackage err = %v, want invalid deb package", err)
	}
}

func TestParseDebPackageRejectsMissingDataArchive(t *testing.T) {
	controlArchive := buildControlArchiveFixture(t, "control.tar.gz", []byte(strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Description: Tiny utilities",
		"",
	}, "\n")))
	deb := buildDebArFixture(t, []debArTestMember{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: "control.tar.gz", data: controlArchive},
	})
	if _, err := ParseDebPackage(deb); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ParseDebPackage err = %v, want invalid deb package", err)
	}
}

func TestParseDebPackageRejectsDataBeforeControl(t *testing.T) {
	controlArchive := buildControlArchiveFixture(t, "control.tar.gz", []byte(strings.Join([]string{
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Description: Tiny utilities",
		"",
	}, "\n")))
	deb := buildDebArFixture(t, []debArTestMember{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: "data.tar", data: nil},
		{name: "control.tar.gz", data: controlArchive},
	})
	if _, err := ParseDebPackage(deb); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ParseDebPackage err = %v, want invalid deb package", err)
	}
}

func TestParseDebPackageRejectsInvalidControl(t *testing.T) {
	deb := buildDebFixture(t, "control.tar.gz", []byte("Package busybox\n"))
	if _, err := ParseDebPackage(deb); !errors.Is(err, ErrInvalidDebPackage) {
		t.Fatalf("ParseDebPackage err = %v, want invalid deb package", err)
	}
}

func buildDebFixture(t *testing.T, controlArchiveName string, control []byte) []byte {
	t.Helper()

	archive := buildControlArchiveFixture(t, controlArchiveName, control)
	return buildDebArFixture(t, []debArTestMember{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: controlArchiveName, data: archive},
		{name: "data.tar", data: nil},
	})
}

func buildControlArchiveFixture(t *testing.T, controlArchiveName string, control []byte) []byte {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "./control",
		Mode: 0o644,
		Size: int64(len(control)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(control); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	switch controlArchiveName {
	case "control.tar":
		archive.Write(tarBuf.Bytes())
	case "control.tar.gz":
		gw := gzip.NewWriter(&archive)
		if _, err := gw.Write(tarBuf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}
	case "control.tar.zst":
		zw, err := zstd.NewWriter(&archive)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := zw.Write(tarBuf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test control archive %s", controlArchiveName)
	}
	return archive.Bytes()
}

type debArTestMember struct {
	name string
	data []byte
}

func buildDebArFixture(t *testing.T, members []debArTestMember) []byte {
	t.Helper()

	var buf bytes.Buffer
	buf.WriteString(debArMagic)
	for _, member := range members {
		name := member.name + "/"
		if len(name) > 16 {
			t.Fatalf("ar member name too long: %s", member.name)
		}
		header := arTestField(name, 16) +
			arTestField("0", 12) +
			arTestField("0", 6) +
			arTestField("0", 6) +
			arTestField("100644", 8) +
			arTestField(strconv.Itoa(len(member.data)), 10) +
			"`\n"
		if len(header) != debArHeaderSize {
			t.Fatalf("ar header size = %d, want %d", len(header), debArHeaderSize)
		}
		buf.WriteString(header)
		if _, err := buf.Write(member.data); err != nil {
			t.Fatal(err)
		}
		if len(member.data)%2 != 0 {
			if err := buf.WriteByte('\n'); err != nil {
				t.Fatal(err)
			}
		}
	}
	return buf.Bytes()
}

func arTestField(value string, width int) string {
	return value + strings.Repeat(" ", width-len(value))
}

func assertStringSlicesEqual(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("slice len = %d, want %d: got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q: got=%v want=%v", i, got[i], want[i], got, want)
		}
	}
}

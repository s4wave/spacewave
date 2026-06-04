package s4wave_apt

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestAptPackageChecksumsRecordsPackagePayloadDigests(t *testing.T) {
	data := []byte("deb-payload")
	checksums := AptPackageChecksums(data)
	assertChecksumHex(t, checksums, "md5", md5Hex(data))
	assertChecksumHex(t, checksums, "sha1", sha1Hex(data))
	assertChecksumHex(t, checksums, "sha256", sha256Hex(data))
}

func TestAptPackagePoolFilename(t *testing.T) {
	pkg := &AptPackage{
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
	}
	filename, err := AptPackagePoolFilename(pkg)
	if err != nil {
		t.Fatalf("AptPackagePoolFilename: %v", err)
	}
	want := "pool/b/busybox/busybox_1.36.1-7_i386.deb"
	if filename != want {
		t.Fatalf("filename = %q, want %q", filename, want)
	}
}

func TestAptPackagePoolFilenameRejectsInvalidPathMetadata(t *testing.T) {
	tests := []struct {
		name string
		pkg  *AptPackage
	}{
		{
			name: "dot package name",
			pkg: &AptPackage{
				Name:         ".",
				Version:      "1.0",
				Architecture: "i386",
			},
		},
		{
			name: "uppercase package name",
			pkg: &AptPackage{
				Name:         "Busybox",
				Version:      "1.0",
				Architecture: "i386",
			},
		},
		{
			name: "slash package name",
			pkg: &AptPackage{
				Name:         "busy/box",
				Version:      "1.0",
				Architecture: "i386",
			},
		},
		{
			name: "path version",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "..",
				Architecture: "i386",
			},
		},
		{
			name: "slash version",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1/2",
				Architecture: "i386",
			},
		},
		{
			name: "underscore version",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1.0_1",
				Architecture: "i386",
			},
		},
		{
			name: "empty debian revision",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1.0-",
				Architecture: "i386",
			},
		},
		{
			name: "invalid epoch version",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "bad:1.0",
				Architecture: "i386",
			},
		},
		{
			name: "multiple epoch separators",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1:2:3",
				Architecture: "i386",
			},
		},
		{
			name: "space architecture",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1.0",
				Architecture: "bad arch",
			},
		},
		{
			name: "underscore architecture",
			pkg: &AptPackage{
				Name:         "busybox",
				Version:      "1.0",
				Architecture: "bad_arch",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AptPackagePoolFilename(test.pkg); !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
				t.Fatalf("AptPackagePoolFilename err = %v, want invalid index metadata", err)
			}
		})
	}
}

func TestGeneratePackagesFileRendersPublishedPackages(t *testing.T) {
	bashPayload := []byte("bash-deb")
	busyboxPayload := []byte("busybox-deb")
	bash := testIndexPackage(t, "bash", "5.2-2", bashPayload)
	busybox := testIndexPackage(t, "busybox", "1:1.36.1-7", busyboxPayload)
	built := testIndexPackage(t, "zlib1g", "1.3-1", []byte("zlib-deb"))
	built.State = AptPackageState_AptPackageState_BUILT

	data, err := GeneratePackagesFile([]*AptPackage{busybox, built, bash})
	if err != nil {
		t.Fatalf("GeneratePackagesFile: %v", err)
	}
	want := strings.Join([]string{
		"Package: bash",
		"Version: 5.2-2",
		"Architecture: i386",
		"Filename: pool/b/bash/bash_5.2-2_i386.deb",
		"Size: 8",
		"MD5sum: " + md5Hex(bashPayload),
		"SHA1: " + sha1Hex(bashPayload),
		"SHA256: " + sha256Hex(bashPayload),
		"Depends: libc6 (>= 2.34)",
		"Provides: sh",
		"Conflicts: bash-static",
		"Description: bash shell",
		" second line",
		" .",
		" third paragraph",
		"",
		"Package: busybox",
		"Version: 1:1.36.1-7",
		"Architecture: i386",
		"Filename: pool/b/busybox/busybox_1.36.1-7_i386.deb",
		"Size: 11",
		"MD5sum: " + md5Hex(busyboxPayload),
		"SHA1: " + sha1Hex(busyboxPayload),
		"SHA256: " + sha256Hex(busyboxPayload),
		"Depends: libc6 (>= 2.34)",
		"Provides: sh",
		"Conflicts: busybox-static",
		"Description: busybox shell",
		" second line",
		" .",
		" third paragraph",
		"",
		"",
	}, "\n")
	if string(data) != want {
		t.Fatalf("Packages file:\n%s\nwant:\n%s", string(data), want)
	}
	if strings.Contains(string(data), "zlib1g") {
		t.Fatal("built package appeared in Packages file")
	}
}

func TestGeneratePackagesFileRejectsDuplicatePoolFilename(t *testing.T) {
	payload := []byte("busybox-deb")
	first := testIndexPackage(t, "busybox", "1:1.0-1", payload)
	second := testIndexPackage(t, "busybox", "2:1.0-1", payload)
	if _, err := GeneratePackagesFile([]*AptPackage{first, second}); !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
		t.Fatalf("GeneratePackagesFile err = %v, want invalid index metadata", err)
	}
}

func TestGeneratePackagesFileRejectsPublishedPackageWithoutSHA256(t *testing.T) {
	pkg := testIndexPackage(t, "busybox", "1.36.1-7", []byte("busybox-deb"))
	pkg.Checksums = pkg.GetChecksums()[:2]
	if _, err := GeneratePackagesFile([]*AptPackage{pkg}); !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
		t.Fatalf("GeneratePackagesFile err = %v, want invalid index metadata", err)
	}
}

func TestGeneratePackagesFileRejectsInvalidChecksumMetadata(t *testing.T) {
	tests := []struct {
		name      string
		checksums []*AptPackageChecksum
	}{
		{
			name: "missing md5",
			checksums: []*AptPackageChecksum{
				{Algorithm: "sha1", Hex: sha1Hex([]byte("busybox-deb"))},
				{Algorithm: "sha256", Hex: sha256Hex([]byte("busybox-deb"))},
			},
		},
		{
			name: "short md5",
			checksums: []*AptPackageChecksum{
				{Algorithm: "md5", Hex: "0123"},
				{Algorithm: "sha1", Hex: sha1Hex([]byte("busybox-deb"))},
				{Algorithm: "sha256", Hex: sha256Hex([]byte("busybox-deb"))},
			},
		},
		{
			name: "non-hex sha256",
			checksums: []*AptPackageChecksum{
				{Algorithm: "md5", Hex: md5Hex([]byte("busybox-deb"))},
				{Algorithm: "sha1", Hex: sha1Hex([]byte("busybox-deb"))},
				{Algorithm: "sha256", Hex: strings.Repeat("z", 64)},
			},
		},
		{
			name: "duplicate sha1",
			checksums: []*AptPackageChecksum{
				{Algorithm: "md5", Hex: md5Hex([]byte("busybox-deb"))},
				{Algorithm: "sha1", Hex: sha1Hex([]byte("busybox-deb"))},
				{Algorithm: "sha1", Hex: sha1Hex([]byte("busybox-deb"))},
				{Algorithm: "sha256", Hex: sha256Hex([]byte("busybox-deb"))},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pkg := testIndexPackage(t, "busybox", "1.36.1-7", []byte("busybox-deb"))
			pkg.Checksums = test.checksums
			if _, err := GeneratePackagesFile([]*AptPackage{pkg}); !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
				t.Fatalf("GeneratePackagesFile err = %v, want invalid index metadata", err)
			}
		})
	}
}

func TestGeneratePackagesGzipFileCompressesPackagesFile(t *testing.T) {
	pkg := testIndexPackage(t, "busybox", "1.36.1-7", []byte("busybox-deb"))
	packagesFile, err := GeneratePackagesFile([]*AptPackage{pkg})
	if err != nil {
		t.Fatalf("GeneratePackagesFile: %v", err)
	}
	compressed, err := GeneratePackagesGzipFile([]*AptPackage{pkg})
	if err != nil {
		t.Fatalf("GeneratePackagesGzipFile: %v", err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	decompressed, err := io.ReadAll(gr)
	if closeErr := gr.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if string(decompressed) != string(packagesFile) {
		t.Fatalf("decompressed Packages.gz = %q, want %q", decompressed, packagesFile)
	}
}

func TestGeneratePackagesFileFromPublishedWorldPackage(t *testing.T) {
	ctx, ws, packageKey := setupAptWorldWithBuiltPackage(t)
	if _, _, err := ws.ApplyWorldOp(ctx, NewAptPublishPackageOp(packageKey), ""); err != nil {
		t.Fatalf("ApplyWorldOp(publish): %v", err)
	}
	pkg := readAptBlock[*AptPackage](t, ctx, ws, packageKey, func() block.Block {
		return &AptPackage{}
	})
	data, err := GeneratePackagesFile([]*AptPackage{pkg})
	if err != nil {
		t.Fatalf("GeneratePackagesFile: %v", err)
	}
	assertContains(t, string(data), "Package: busybox\n")
	assertContains(t, string(data), "SHA256: ")
}

func testIndexPackage(t *testing.T, name string, version string, payload []byte) *AptPackage {
	t.Helper()

	ref, err := block.BuildBlockRef(payload, nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	return &AptPackage{
		State:        AptPackageState_AptPackageState_PUBLISHED,
		Name:         name,
		Version:      version,
		Architecture: "i386",
		Depends:      []string{"libc6 (>= 2.34)"},
		Provides:     []string{"sh"},
		Conflicts:    []string{name + "-static"},
		Description:  name + " shell\nsecond line\n\nthird paragraph",
		Size:         uint64(len(payload)),
		Checksums:    AptPackageChecksums(payload),
		DebRef:       ref,
	}
}

func assertChecksumHex(t *testing.T, checksums []*AptPackageChecksum, algorithm string, want string) {
	t.Helper()

	for _, checksum := range checksums {
		if checksum.GetAlgorithm() == algorithm {
			if checksum.GetHex() != want {
				t.Fatalf("%s checksum = %q, want %q", algorithm, checksum.GetHex(), want)
			}
			return
		}
	}
	t.Fatalf("%s checksum not found", algorithm)
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func sha1Hex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertContains(t *testing.T, text string, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Fatalf("expected %q to contain %q", text, want)
	}
}

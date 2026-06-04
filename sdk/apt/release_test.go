package s4wave_apt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGenerateReleaseFileRendersRepositoryMetadataAndChecksums(t *testing.T) {
	packagesFile := []byte("Package: busybox\n\n")
	packagesGzip := []byte("gzip-data")
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}
	date := time.Date(2026, 5, 20, 21, 30, 0, 0, time.UTC)
	data, err := GenerateReleaseFile(repo, map[string][]byte{
		"main/binary-i386/Packages.gz": packagesGzip,
		"main/binary-i386/Packages":    packagesFile,
	}, date)
	if err != nil {
		t.Fatalf("GenerateReleaseFile: %v", err)
	}
	want := strings.Join([]string{
		"Suite: stable",
		"Codename: stable",
		"Date: Wed, 20 May 2026 21:30:00 UTC",
		"Architectures: i386",
		"Components: main",
		"MD5Sum:",
		" " + md5Hex(packagesFile) + " 18 main/binary-i386/Packages",
		" " + md5Hex(packagesGzip) + " 9 main/binary-i386/Packages.gz",
		"SHA1:",
		" " + sha1Hex(packagesFile) + " 18 main/binary-i386/Packages",
		" " + sha1Hex(packagesGzip) + " 9 main/binary-i386/Packages.gz",
		"SHA256:",
		" " + sha256Hex(packagesFile) + " 18 main/binary-i386/Packages",
		" " + sha256Hex(packagesGzip) + " 9 main/binary-i386/Packages.gz",
		"",
	}, "\n")
	if string(data) != want {
		t.Fatalf("Release file:\n%s\nwant:\n%s", string(data), want)
	}
}

func TestGenerateReleaseFileRejectsEscapingPath(t *testing.T) {
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}
	_, err := GenerateReleaseFile(repo, map[string][]byte{"../Packages": []byte("x")}, time.Now())
	if !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
		t.Fatalf("GenerateReleaseFile err = %v, want invalid index metadata", err)
	}
}

func TestGenerateReleaseFileRejectsControlCharacterPath(t *testing.T) {
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}
	_, err := GenerateReleaseFile(repo, map[string][]byte{"main/binary-i386/Packages\nSHA256: injected": []byte("x")}, time.Now())
	if !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
		t.Fatalf("GenerateReleaseFile err = %v, want invalid index metadata", err)
	}
}

func TestGenerateReleaseFileRejectsDuplicateCleanPath(t *testing.T) {
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}
	_, err := GenerateReleaseFile(repo, map[string][]byte{
		"main/binary-i386/Packages":   []byte("a"),
		"main/./binary-i386/Packages": []byte("b"),
	}, time.Now())
	if !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
		t.Fatalf("GenerateReleaseFile err = %v, want invalid index metadata", err)
	}
}

func TestGenerateReleaseFileRejectsInvalidRepositoryMetadata(t *testing.T) {
	tests := []struct {
		name string
		repo *AptRepository
	}{
		{
			name: "distribution path",
			repo: &AptRepository{
				State:         AptRepositoryState_AptRepositoryState_EMPTY,
				Distribution:  "stable/updates",
				Components:    []string{"main"},
				Architectures: []string{"i386"},
			},
		},
		{
			name: "component whitespace",
			repo: &AptRepository{
				State:         AptRepositoryState_AptRepositoryState_EMPTY,
				Distribution:  "stable",
				Components:    []string{"main contrib"},
				Architectures: []string{"i386"},
			},
		},
		{
			name: "architecture control character",
			repo: &AptRepository{
				State:         AptRepositoryState_AptRepositoryState_EMPTY,
				Distribution:  "stable",
				Components:    []string{"main"},
				Architectures: []string{"i386\nSHA256"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := GenerateReleaseFile(test.repo, map[string][]byte{"main/binary-i386/Packages": []byte("x")}, time.Now())
			if !errors.Is(err, ErrInvalidAptPackageIndexMetadata) {
				t.Fatalf("GenerateReleaseFile err = %v, want invalid index metadata", err)
			}
		})
	}
}

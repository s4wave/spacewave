package s4wave_apt

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

func TestAptPackageTypeIDPredicatesAndBlock(t *testing.T) {
	if AptPackageTypeID != "spacewave-vm/apt/package" {
		t.Fatalf("AptPackageTypeID = %q", AptPackageTypeID)
	}
	predicates := map[string]string{
		"repo-package":      string(PredAptRepoPackage),
		"repo-buildspec":    string(PredAptRepoBuildSpec),
		"package-buildspec": string(PredAptPackageBuildSpec),
	}
	expected := map[string]string{
		"repo-package":      "spacewave-vm/apt/repo-package",
		"repo-buildspec":    "spacewave-vm/apt/repo-buildspec",
		"package-buildspec": "spacewave-vm/apt/package-buildspec",
	}
	for name, got := range predicates {
		if got != expected[name] {
			t.Fatalf("%s predicate = %q, want %q", name, got, expected[name])
		}
	}

	ref, err := block.BuildBlockRef([]byte("deb-payload"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	pkg := &AptPackage{
		State:        AptPackageState_AptPackageState_BUILT,
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
		Depends:      []string{"libc6 (>= 2.34)"},
		Provides:     []string{"busybox-static"},
		Conflicts:    []string{"busybox-cvs"},
		Description:  "Tiny utilities for small systems",
		Size:         524288,
		Checksums: []*AptPackageChecksum{
			{Algorithm: "sha256", Hex: "0123456789abcdef"},
		},
		DebRef: ref,
	}
	if got := pkg.GetBlockTypeId(); got != AptPackageTypeID {
		t.Fatalf("AptPackage block type = %q, want %q", got, AptPackageTypeID)
	}
	if err := pkg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	data, err := pkg.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	var decoded AptPackage
	if err := decoded.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}
	if !decoded.EqualVT(pkg) {
		t.Fatalf("decoded package does not match original: got=%s want=%s", decoded.String(), pkg.String())
	}
}

func TestAptPackageValidateRejectsMissingCoreFields(t *testing.T) {
	ref, err := block.BuildBlockRef([]byte("deb-payload"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	if err := (&AptPackage{}).Validate(); err == nil {
		t.Fatal("expected missing name error")
	}
	if err := (&AptPackage{Name: "busybox"}).Validate(); err == nil {
		t.Fatal("expected missing version error")
	}
	if err := (&AptPackage{
		Name:    "busybox",
		Version: "1:1.36.1-7",
	}).Validate(); err == nil {
		t.Fatal("expected missing architecture error")
	}
	if err := (&AptPackage{
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
	}).Validate(); err == nil {
		t.Fatal("expected missing deb_ref error")
	}
	if err := (&AptPackage{
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
		DebRef:       ref,
		Checksums:    []*AptPackageChecksum{{Hex: "0123"}},
	}).Validate(); err == nil {
		t.Fatal("expected missing checksum algorithm error")
	}
	if err := (&AptPackage{
		Name:         "busybox",
		Version:      "1:1.36.1-7",
		Architecture: "i386",
		DebRef:       ref,
		Checksums:    []*AptPackageChecksum{{Algorithm: "sha256"}},
	}).Validate(); err == nil {
		t.Fatal("expected missing checksum hex error")
	}
}

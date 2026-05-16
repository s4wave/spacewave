package s4wave_apt

import "testing"

func TestAptRepositoryTypeIDAndBlock(t *testing.T) {
	if AptRepositoryTypeID != "spacewave-vm/apt/repository" {
		t.Fatalf("AptRepositoryTypeID = %q", AptRepositoryTypeID)
	}

	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_READY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}
	if got := repo.GetBlockTypeId(); got != AptRepositoryTypeID {
		t.Fatalf("AptRepository block type = %q, want %q", got, AptRepositoryTypeID)
	}
	if err := repo.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	data, err := repo.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock: %v", err)
	}
	var decoded AptRepository
	if err := decoded.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock: %v", err)
	}
	if !decoded.EqualVT(repo) {
		t.Fatalf("decoded repository does not match original: got=%s want=%s", decoded.String(), repo.String())
	}
}

func TestAptRepositoryValidateRejectsMissingCoreFields(t *testing.T) {
	if err := (&AptRepository{}).Validate(); err == nil {
		t.Fatal("expected missing distribution error")
	}
	if err := (&AptRepository{Distribution: "stable"}).Validate(); err == nil {
		t.Fatal("expected missing components error")
	}
	if err := (&AptRepository{
		Distribution: "stable",
		Components:   []string{"main"},
	}).Validate(); err == nil {
		t.Fatal("expected missing architectures error")
	}
}

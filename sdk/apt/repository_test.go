package s4wave_apt

import (
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

func TestAptRepositoryTypeIDAndBlock(t *testing.T) {
	// Verify the repository type identifier.
	if AptRepositoryTypeID != "spacewave-vm/apt/repository" {
		t.Fatalf("AptRepositoryTypeID = %q", AptRepositoryTypeID)
	}

	// Prepare a ready repository and verify its block type and validity.
	indexRef, err := block.BuildBlockRef([]byte("dists-index"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_READY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
		IndexRef:      &bucket.ObjectRef{RootRef: indexRef},
	}
	if got := repo.GetBlockTypeId(); got != AptRepositoryTypeID {
		t.Fatalf("AptRepository block type = %q, want %q", got, AptRepositoryTypeID)
	}
	if err := repo.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Verify the repository block survives binary encoding and decoding.
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

func TestAptRepositoryStateTransitions(t *testing.T) {
	// Prepare an empty repository with an available index reference.
	indexRef, err := block.BuildBlockRef([]byte("dists-index"), nil)
	if err != nil {
		t.Fatalf("BuildBlockRef: %v", err)
	}
	repo := &AptRepository{
		State:         AptRepositoryState_AptRepositoryState_EMPTY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}

	// Index the repository and verify its indexing state.
	if err := repo.TransitionState(AptRepositoryState_AptRepositoryState_INDEXING); err != nil {
		t.Fatalf("TransitionState(indexing): %v", err)
	}
	if got := repo.GetState(); got != AptRepositoryState_AptRepositoryState_INDEXING {
		t.Fatalf("repository state = %s, want INDEXING", got.String())
	}

	// Attach the index and verify the repository becomes ready.
	repo.IndexRef = &bucket.ObjectRef{RootRef: indexRef}
	if err := repo.TransitionState(AptRepositoryState_AptRepositoryState_READY); err != nil {
		t.Fatalf("TransitionState(ready): %v", err)
	}
	if got := repo.GetState(); got != AptRepositoryState_AptRepositoryState_READY {
		t.Fatalf("repository state = %s, want READY", got.String())
	}

	// Verify error and reindex transitions follow the repository state rules.
	if err := repo.TransitionState(AptRepositoryState_AptRepositoryState_ERROR); !errors.Is(err, ErrInvalidAptRepositoryStateTransition) {
		t.Fatalf("TransitionState(error) err = %v, want invalid transition", err)
	}
	if err := repo.TransitionState(AptRepositoryState_AptRepositoryState_INDEXING); err != nil {
		t.Fatalf("TransitionState(reindex): %v", err)
	}
	if err := repo.TransitionState(AptRepositoryState_AptRepositoryState_ERROR); err != nil {
		t.Fatalf("TransitionState(error after indexing): %v", err)
	}
}

func TestAptRepositoryValidateRejectsMissingCoreFields(t *testing.T) {
	// Verify incomplete repositories and unknown states are rejected.
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
	if err := (&AptRepository{
		State:         AptRepositoryState_AptRepositoryState_READY,
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}).Validate(); err == nil {
		t.Fatal("expected missing index_ref error")
	}
	if err := (&AptRepository{
		State:         AptRepositoryState(99),
		Distribution:  "stable",
		Components:    []string{"main"},
		Architectures: []string{"i386"},
	}).Validate(); !errors.Is(err, ErrUnknownAptRepositoryState) {
		t.Fatalf("Validate unknown state err = %v, want unknown state", err)
	}
}

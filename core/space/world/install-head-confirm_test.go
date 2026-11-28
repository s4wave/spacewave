package space_world

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestInstallHeadConfirmsChangesBeforeReporting keeps evidence unconfirmed on a
// failed retained-root hold and confirms it before a successful head is visible.
func TestInstallHeadConfirmsChangesBeforeReporting(t *testing.T) {
	// Use real block storage with the first retained-root swap failing.
	ctx := t.Context()
	store := &flakyRootStore{StoreOps: block_mock.NewMockStore(0), roots: make(map[string]*block.BlockRef), fail: 1}
	head, err := block.BuildBlockRef([]byte("confirmed head"), nil)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := block.BuildBlockRef([]byte("retained root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &Adapter{
		moves: newRootMoveQueue(false), opened: openedNow(),
		le: logrus.NewEntry(logrus.New()), pages: block.NewPendingStore(store, 1<<20),
		held: make(map[string]*block.BlockRef),
	}
	a.replay = &spaceReplay{a: a, stage: newReplayStage(a.pages), base: &InnerState{
		HeadRef:       &bucket.ObjectRef{RootRef: head},
		RetainedRoots: []*RetainedRoot{{Name: "export", RootRef: retained}},
	}}

	// The admitted callback observes the reportable head under the same lock.
	confirmed := false
	adm := &spaceAdmission{state: admissionCovered, done: make(chan struct{}), candidate: world.Candidate{
		ConfirmDurable: func(got world.Head) {
			if !got.Root.EqualsRef(head) || !got.Durable {
				t.Fatalf("confirmed head %v, want durable %v", got, head)
			}
			if !confirmed && !a.reportable.Root.GetEmpty() {
				t.Fatal("head became reportable before change confirmation")
			}
			confirmed = true
		},
	}}
	a.pending = []*spaceAdmission{adm}

	// A failed durability prerequisite cannot confirm or expose the head.
	if err := a.installHead(ctx); err == nil {
		t.Fatal("first install bypassed the failed retained-root hold")
	}
	if confirmed || !a.reportable.Root.GetEmpty() || adm.state == admissionDecided {
		t.Fatal("failed hold confirmed evidence or published the head")
	}

	// Retrying the hold confirms evidence before making the head reportable.
	if err := a.installHead(ctx); err != nil {
		t.Fatal(err)
	}
	if !confirmed || !a.reportable.Root.EqualsRef(head) || adm.state != admissionDecided {
		t.Fatal("successful install did not confirm and publish its head")
	}
}

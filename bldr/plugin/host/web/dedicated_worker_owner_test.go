package plugin_host_web

import "testing"

func TestDedicatedWorkerOwnerKeepsFirstLiveDedicatedWorker(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Create the first dedicated worker and verify its running ownership.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	if got := owner.currentState(); got != dedicatedWorkerStateCreatingOwnerWorker {
		t.Fatalf("state after begin create = %v, want creating", got)
	}
	owner.observeCreatedWorker("doc-a", false)
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerRunning {
		t.Fatalf("state after created worker = %v, want owner running", got)
	}

	// Verify a second document cannot replace the live owner.
	if wake := owner.observeDocumentStatus("doc-b", false); wake {
		t.Fatal("visible second document should not wake trackers while owner is alive")
	}
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create = %v, wake = %v; want blocked", create, wake)
	}

	// Verify the owning document can recreate its worker.
	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner document recreate = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerClosedOwnerReleasesSingleton(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Create the first document worker before removing its owner.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	// Remove the owner and verify another document can claim the singleton.
	if wake := owner.observeDocumentRemoved("doc-a"); !wake {
		t.Fatal("removed owner should wake other document trackers")
	}
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerClosed {
		t.Fatalf("state after owner removal = %v, want owner closed", got)
	}
	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create = %v, wake = %v; want create without transfer wake", create, wake)
	}
	owner.observeCreatedWorker("doc-b", false)

	// Verify the removed document cannot replace the new owner.
	create, wake = owner.beginCreate("doc-a")
	if create || wake {
		t.Fatalf("removed first document create = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenOwnerDoesNotTransferSingleton(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Create the first dedicated worker before hiding its document.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	// Hide the owner and verify another document remains blocked.
	if wake := owner.observeDocumentStatus("doc-a", true); wake {
		t.Fatal("hidden owner should not wake other documents")
	}
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerHidden {
		t.Fatalf("state after owner hidden = %v, want owner hidden", got)
	}
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("visible second document create = %v, wake = %v; want blocked while hidden owner is alive", create, wake)
	}

	// Restore the owning document and verify it can recreate its worker.
	if wake := owner.observeDocumentStatus("doc-a", false); wake {
		t.Fatal("visible owner should not wake other documents")
	}
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerRunning {
		t.Fatalf("state after owner visible = %v, want owner running", got)
	}
	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner document recreate = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerWorkerFailureDoesNotTransferSingleton(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Fail the running worker and verify the document retains ownership.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)
	owner.observeWorkerFailed("doc-a")
	if got := owner.currentState(); got != dedicatedWorkerStateFailed {
		t.Fatalf("state after worker failure = %v, want failed", got)
	}

	// Worker failure is reported by the tracker; the singleton owner changes only
	// when the owning document is removed.
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("visible second document create = %v, wake = %v; want blocked after owner worker failure", create, wake)
	}
	if wake := owner.observeDocumentRemoved("doc-a"); !wake {
		t.Fatal("removed failed owner should wake other document trackers")
	}
	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create after failed owner removal = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerSharedWorkersDoNotPinDocument(t *testing.T) {
	// Prepare an unclaimed worker owner.
	var owner dedicatedWorkerOwner

	// Create a shared worker and verify it leaves dedicated ownership unclaimed.
	create, _ := owner.beginCreate("doc-a")
	if !create {
		t.Fatal("first document should create")
	}
	owner.observeCreatedWorker("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after shared worker = %v, want no docs", got)
	}

	// Verify another document can create after a shared worker.
	create, wake := owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("shared worker should not pin owner: create = %v, wake = %v", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenCreateDoesNotPinDocument(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Skip a hidden creation and verify the singleton remains unclaimed.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateSkipped("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after hidden create skip = %v, want no visible docs", got)
	}

	// Verify another document can create after the hidden creation is skipped.
	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create after hidden skip = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenRecreateKeepsOwnerPinned(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Create the first document worker before attempting recreation.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	// Skip recreation while hidden and verify the original owner remains pinned.
	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner recreate = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateSkipped("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerHidden {
		t.Fatalf("state after hidden owner recreate skip = %v, want owner hidden", got)
	}

	// Verify another document cannot replace the hidden owner.
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create after owner hidden skip = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerCreateFailureDoesNotPinDocument(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Fail initial worker creation and verify ownership remains unclaimed.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateFailed("doc-a")
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after create failure = %v, want no docs", got)
	}

	// Verify another document can create after the initial failure.
	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create after create failure = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerRecreateFailureKeepsOwnerPinned(t *testing.T) {
	// Prepare an unclaimed dedicated-worker owner.
	var owner dedicatedWorkerOwner

	// Create the first document worker before attempting recreation.
	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	// Fail recreation and verify the running document retains ownership.
	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner recreate = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateFailed("doc-a")
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerRunning {
		t.Fatalf("state after owner recreate failure = %v, want owner running", got)
	}

	// Verify another document cannot replace the owner after recreation fails.
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create after owner recreate failure = %v, wake = %v; want blocked", create, wake)
	}
}

package plugin_host_web

import "testing"

func TestDedicatedWorkerOwnerKeepsFirstLiveDedicatedWorker(t *testing.T) {
	var owner dedicatedWorkerOwner

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

	if wake := owner.observeDocumentStatus("doc-b", false); wake {
		t.Fatal("visible second document should not wake trackers while owner is alive")
	}
	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create = %v, wake = %v; want blocked", create, wake)
	}

	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner document recreate = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerClosedOwnerReleasesSingleton(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

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

	create, wake = owner.beginCreate("doc-a")
	if create || wake {
		t.Fatalf("removed first document create = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenOwnerDoesNotTransferSingleton(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

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
	var owner dedicatedWorkerOwner

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
	var owner dedicatedWorkerOwner

	create, _ := owner.beginCreate("doc-a")
	if !create {
		t.Fatal("first document should create")
	}
	owner.observeCreatedWorker("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after shared worker = %v, want no docs", got)
	}

	create, wake := owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("shared worker should not pin owner: create = %v, wake = %v", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenCreateDoesNotPinDocument(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateSkipped("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after hidden create skip = %v, want no visible docs", got)
	}

	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create after hidden skip = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenRecreateKeepsOwnerPinned(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner recreate = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateSkipped("doc-a", true)
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerHidden {
		t.Fatalf("state after hidden owner recreate skip = %v, want owner hidden", got)
	}

	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create after owner hidden skip = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerCreateFailureDoesNotPinDocument(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateFailed("doc-a")
	if got := owner.currentState(); got != dedicatedWorkerStateNoDocs {
		t.Fatalf("state after create failure = %v, want no docs", got)
	}

	create, wake = owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create after create failure = %v, wake = %v; want create without wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerRecreateFailureKeepsOwnerPinned(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	create, wake = owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("owner recreate = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreateFailed("doc-a")
	if got := owner.currentState(); got != dedicatedWorkerStateOwnerRunning {
		t.Fatalf("state after owner recreate failure = %v, want owner running", got)
	}

	create, wake = owner.beginCreate("doc-b")
	if create || wake {
		t.Fatalf("second document create after owner recreate failure = %v, wake = %v; want blocked", create, wake)
	}
}

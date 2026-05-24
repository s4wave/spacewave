package plugin_host_web

import "testing"

func TestDedicatedWorkerOwnerTransfersToMostRecentVisibleDocument(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

	if wake := owner.observeDocumentStatus("doc-b", false); !wake {
		t.Fatal("visible second document should wake trackers to transfer ownership")
	}
	create, wake = owner.beginCreate("doc-b")
	if !create || !wake {
		t.Fatalf("preferred document create = %v, wake = %v; want create with wake", create, wake)
	}
	owner.observeCreatedWorker("doc-b", false)

	create, wake = owner.beginCreate("doc-a")
	if create || wake {
		t.Fatalf("stale first document create = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerDoesNotOscillateBetweenAlreadyVisibleDocuments(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)
	owner.observeDocumentStatus("doc-a", false)

	if wake := owner.observeDocumentStatus("doc-b", false); !wake {
		t.Fatal("newly visible second document should wake first document")
	}
	create, wake = owner.beginCreate("doc-b")
	if !create || !wake {
		t.Fatalf("second document create = %v, wake = %v; want create with wake", create, wake)
	}
	owner.observeCreatedWorker("doc-b", false)

	if wake := owner.observeDocumentStatus("doc-a", false); wake {
		t.Fatal("already-visible first document should not steal ownership back")
	}
	create, wake = owner.beginCreate("doc-a")
	if create || wake {
		t.Fatalf("already-visible first document create = %v, wake = %v; want blocked", create, wake)
	}
}

func TestDedicatedWorkerOwnerHiddenOwnerReleasesSingleton(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, _ := owner.beginCreate("doc-a")
	if !create {
		t.Fatal("first document should create")
	}
	owner.observeCreatedWorker("doc-a", false)

	if wake := owner.observeDocumentStatus("doc-a", true); !wake {
		t.Fatal("hidden owner should wake other document trackers")
	}
	create, wake := owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("second document create = %v, wake = %v; want create without transfer wake", create, wake)
	}
}

func TestDedicatedWorkerOwnerSharedWorkersDoNotPinDocument(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, _ := owner.beginCreate("doc-a")
	if !create {
		t.Fatal("first document should create")
	}
	owner.observeCreatedWorker("doc-a", true)

	create, wake := owner.beginCreate("doc-b")
	if !create || wake {
		t.Fatalf("shared worker should not pin owner: create = %v, wake = %v", create, wake)
	}
}

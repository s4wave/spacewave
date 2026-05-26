package plugin_host_web

import "testing"

func TestDedicatedWorkerOwnerKeepsFirstLiveDedicatedWorker(t *testing.T) {
	var owner dedicatedWorkerOwner

	create, wake := owner.beginCreate("doc-a")
	if !create || wake {
		t.Fatalf("first document create = %v, wake = %v; want create without wake", create, wake)
	}
	owner.observeCreatedWorker("doc-a", false)

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

package plugin_host_web

// dedicatedWorkerOwner tracks which WebDocument owns a dedicated plugin worker.
//
// Config B/C browsers create dedicated plugin workers for SAB-backed intra-tab
// IPC. Those workers have a singleton runtime client id, so only one document can
// own a given plugin worker at a time. The owner remains stable while its
// document is alive: other tabs share the same worker through the WebRuntime
// instead of creating a replacement instance. If the owner document closes, the
// next visible document can create the replacement worker.
type dedicatedWorkerOwner struct {
	ownerDocID string
	state      dedicatedWorkerState
	docs       map[string]bool
}

type dedicatedWorkerState uint8

const (
	dedicatedWorkerStateNoDocs dedicatedWorkerState = iota
	dedicatedWorkerStateVisibleDocsNoOwner
	dedicatedWorkerStateCreatingOwnerWorker
	dedicatedWorkerStateOwnerRunning
	dedicatedWorkerStateOwnerHidden
	dedicatedWorkerStateOwnerClosed
	dedicatedWorkerStateFailed
)

func (o *dedicatedWorkerOwner) observeDocumentStatus(docID string, hidden bool) (wake bool) {
	o.ensureDocs()
	o.docs[docID] = hidden
	if o.ownerDocID != docID {
		o.refreshIdleState()
		return false
	}
	if o.state == dedicatedWorkerStateFailed {
		return false
	}
	o.refreshOwnerState()
	return false
}

func (o *dedicatedWorkerOwner) observeDocumentRemoved(docID string) (wake bool) {
	o.ensureDocs()
	delete(o.docs, docID)
	if o.ownerDocID == docID {
		o.ownerDocID = ""
		o.state = dedicatedWorkerStateOwnerClosed
		return true
	}
	o.refreshIdleState()
	return false
}

func (o *dedicatedWorkerOwner) beginCreate(docID string) (create bool, wake bool) {
	if o.ownerDocID == docID {
		return true, false
	}
	if o.ownerDocID == "" {
		o.ownerDocID = docID
		o.state = dedicatedWorkerStateCreatingOwnerWorker
		return true, false
	}
	return false, false
}

func (o *dedicatedWorkerOwner) observeCreateSkipped(docID string, hidden bool) {
	o.ensureDocs()
	o.docs[docID] = hidden
	if o.ownerDocID == docID && o.state == dedicatedWorkerStateCreatingOwnerWorker {
		o.ownerDocID = ""
	}
	if o.ownerDocID == docID {
		o.refreshOwnerState()
		return
	}
	o.refreshIdleState()
}

func (o *dedicatedWorkerOwner) observeCreateFailed(docID string) {
	if o.ownerDocID == docID && o.state == dedicatedWorkerStateCreatingOwnerWorker {
		o.ownerDocID = ""
	}
	if o.ownerDocID == docID {
		o.refreshOwnerState()
		return
	}
	o.refreshIdleState()
}

func (o *dedicatedWorkerOwner) observeCreatedWorker(docID string, shared bool) {
	if shared {
		if o.ownerDocID == docID {
			o.ownerDocID = ""
		}
		o.refreshIdleState()
		return
	}
	o.ownerDocID = docID
	o.refreshOwnerState()
}

func (o *dedicatedWorkerOwner) observeWorkerFailed(docID string) {
	if o.ownerDocID == docID {
		o.state = dedicatedWorkerStateFailed
	}
}

func (o *dedicatedWorkerOwner) currentState() dedicatedWorkerState {
	if o.ownerDocID == "" && o.state != dedicatedWorkerStateOwnerClosed {
		o.refreshIdleState()
	}
	return o.state
}

func (o *dedicatedWorkerOwner) ensureDocs() {
	if o.docs == nil {
		o.docs = make(map[string]bool)
	}
}

func (o *dedicatedWorkerOwner) refreshOwnerState() {
	o.ensureDocs()
	if o.ownerDocID == "" {
		o.refreshIdleState()
		return
	}
	if o.docs[o.ownerDocID] {
		o.state = dedicatedWorkerStateOwnerHidden
		return
	}
	o.state = dedicatedWorkerStateOwnerRunning
}

func (o *dedicatedWorkerOwner) refreshIdleState() {
	o.ensureDocs()
	if o.ownerDocID != "" {
		return
	}
	for _, hidden := range o.docs {
		if !hidden {
			o.state = dedicatedWorkerStateVisibleDocsNoOwner
			return
		}
	}
	o.state = dedicatedWorkerStateNoDocs
}

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
}

func (o *dedicatedWorkerOwner) observeDocumentStatus(docID string, hidden bool) (wake bool) {
	return false
}

func (o *dedicatedWorkerOwner) observeDocumentRemoved(docID string) (wake bool) {
	if o.ownerDocID == docID {
		o.ownerDocID = ""
		return true
	}
	return false
}

func (o *dedicatedWorkerOwner) beginCreate(docID string) (create bool, wake bool) {
	if o.ownerDocID == docID {
		return true, false
	}
	if o.ownerDocID == "" {
		return true, false
	}
	return false, false
}

func (o *dedicatedWorkerOwner) observeCreatedWorker(docID string, shared bool) {
	if shared {
		if o.ownerDocID == docID {
			o.ownerDocID = ""
		}
		return
	}
	o.ownerDocID = docID
}

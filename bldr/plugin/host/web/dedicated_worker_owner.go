package plugin_host_web

// dedicatedWorkerOwner tracks which WebDocument owns a dedicated plugin worker.
//
// Config B/C browsers create dedicated plugin workers for SAB-backed intra-tab
// IPC. Those workers have a singleton runtime client id, so only one document can
// own a given plugin worker at a time. Ownership follows the most recently
// visible document; otherwise a first-opened hidden tab can keep later tabs stuck
// before their app worker ever reaches the WebView.
type dedicatedWorkerOwner struct {
	ownerDocID     string
	preferredDocID string
	docHidden      map[string]bool
}

func (o *dedicatedWorkerOwner) observeDocumentStatus(docID string, hidden bool) (wake bool) {
	if o.docHidden == nil {
		o.docHidden = make(map[string]bool)
	}
	wasHidden, known := o.docHidden[docID]
	o.docHidden[docID] = hidden
	if hidden {
		if o.preferredDocID == docID {
			o.preferredDocID = ""
		}
		if o.ownerDocID == docID {
			o.ownerDocID = ""
			return true
		}
		return false
	}

	becamePreferred := !known || wasHidden
	if becamePreferred {
		o.preferredDocID = docID
	}
	return becamePreferred && o.ownerDocID != "" && o.ownerDocID != docID
}

func (o *dedicatedWorkerOwner) observeDocumentRemoved(docID string) (wake bool) {
	if o.docHidden != nil {
		delete(o.docHidden, docID)
	}
	if o.preferredDocID == docID {
		o.preferredDocID = ""
	}
	if o.ownerDocID == docID {
		o.ownerDocID = ""
		return true
	}
	return false
}

func (o *dedicatedWorkerOwner) beginCreate(docID string) (create bool, wake bool) {
	if o.ownerDocID == docID {
		o.ownerDocID = ""
		return true, true
	}
	if o.ownerDocID == "" {
		return true, false
	}
	if o.preferredDocID == docID {
		o.ownerDocID = ""
		return true, true
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

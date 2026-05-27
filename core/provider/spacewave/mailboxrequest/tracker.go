package mailboxrequest

type key struct {
	soID     string
	inviteID string
	peerID   string
}

// Tracker tracks invitee-visible mailbox request decisions.
type Tracker struct {
	status map[key]string
}

// Track stores the current status for a mailbox request.
func (t *Tracker) Track(soID string, inviteID string, peerID string, status string) bool {
	if soID == "" || inviteID == "" || peerID == "" || status == "" {
		return false
	}
	if t.status == nil {
		t.status = make(map[key]string)
	}
	reqKey := key{
		soID:     soID,
		inviteID: inviteID,
		peerID:   peerID,
	}
	if t.status[reqKey] == status {
		return false
	}
	t.status[reqKey] = status
	return true
}

// Decision returns the terminal decision for a mailbox request.
func (t *Tracker) Decision(soID string, inviteID string, peerID string) (string, bool) {
	if soID == "" || inviteID == "" || peerID == "" || t.status == nil {
		return "", false
	}
	status := t.status[key{
		soID:     soID,
		inviteID: inviteID,
		peerID:   peerID,
	}]
	if status == "" || status == "pending" {
		return "", false
	}
	return status, true
}

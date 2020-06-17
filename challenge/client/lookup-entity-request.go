package auth_challenge_client

import auth_challenge "github.com/aperturerobotics/auth/challenge"

type lookupEntityResultCb func(*auth_challenge.EntityLookupFinish)

// lookupEntityRequest is a handle to an ongoing request.
type lookupEntityRequest struct {
	rid      auth_challenge.RequestID
	entityID string
	domainID string

	// controller mtx guards below fields

	// cbs is callback map
	// refcount == len(cbs)
	cbs map[int]lookupEntityResultCb
	// cbnonce is the callback nonce
	cbnonce int

	// result contains the result if it is completed already
	result *auth_challenge.EntityLookupFinish
}

// newLookupEntityRequest constructs a new request object.
func newLookupEntityRequest(domainID, entityID string, cb lookupEntityResultCb) *lookupEntityRequest {
	return &lookupEntityRequest{
		rid:      auth_challenge.NewRequestID(domainID, entityID),
		entityID: entityID,
		domainID: domainID,
		cbnonce:  1,
		cbs: map[int]lookupEntityResultCb{
			0: cb,
		},
	}
}

// addRefcount adds a reference to the request.
// expects mtx to be locked on controller
// returns ref id
func (r *lookupEntityRequest) addRefcount(cb lookupEntityResultCb) int {
	if r.result != nil {
		cb(r.result)
		return -1
	}

	n := r.cbnonce
	r.cbs[n] = cb
	r.cbnonce++
	return n
}

// decRefcount decrements the refcount.
// expects mtx to be locked on controller
func (r *lookupEntityRequest) decRefcount(id int) {
	if id == -1 {
		return
	}
	delete(r.cbs, id)
}

// applyResult applies the result.
// expects mtx to be locked on controller
func (r *lookupEntityRequest) applyResult(res *auth_challenge.EntityLookupFinish) {
	r.result = res
	for id, cb := range r.cbs {
		delete(r.cbs, id)
		if cb != nil {
			cb(res)
		}
	}
}

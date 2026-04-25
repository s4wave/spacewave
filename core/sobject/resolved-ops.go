package sobject

import "slices"

// FilterResolvedOperations removes pending ops already resolved by root
// account nonces, explicitly accepted operations, or recorded rejections.
// acceptedOps may be nil when the caller only needs root/rejection pruning.
func FilterResolvedOperations(
	ops []*SOOperation,
	accountNonces []*SOAccountNonce,
	acceptedOps []*SOOperationInner,
	rejections []*SOPeerOpRejections,
) []*SOOperation {
	return filterResolvedOperations(
		ops,
		buildAcceptedOperationNonceMap(accountNonces, acceptedOps),
		buildRejectedOperationNonceMap(rejections),
	)
}

func buildAcceptedOperationNonceMap(
	accountNonces []*SOAccountNonce,
	acceptedOps []*SOOperationInner,
) map[string]uint64 {
	accepted := make(map[string]uint64, len(accountNonces))
	for _, nonce := range accountNonces {
		if nonce == nil {
			continue
		}
		peerID := nonce.GetPeerId()
		if peerID == "" {
			continue
		}
		if curr, ok := accepted[peerID]; !ok || nonce.GetNonce() > curr {
			accepted[peerID] = nonce.GetNonce()
		}
	}
	for _, op := range acceptedOps {
		if op == nil {
			continue
		}
		peerID := op.GetPeerId()
		if peerID == "" {
			continue
		}
		if curr, ok := accepted[peerID]; !ok || op.GetNonce() > curr {
			accepted[peerID] = op.GetNonce()
		}
	}
	return accepted
}

func buildRejectedOperationNonceMap(rejections []*SOPeerOpRejections) map[string]map[uint64]bool {
	rejected := make(map[string]map[uint64]bool, len(rejections))
	for _, peerRejections := range rejections {
		if peerRejections == nil {
			continue
		}
		peerID := peerRejections.GetPeerId()
		if peerID == "" {
			continue
		}
		for _, rejection := range peerRejections.GetRejections() {
			if rejection == nil {
				continue
			}
			inner, err := rejection.UnmarshalInner()
			if err != nil {
				continue
			}
			peerRejected := rejected[peerID]
			if peerRejected == nil {
				peerRejected = make(map[uint64]bool)
				rejected[peerID] = peerRejected
			}
			peerRejected[inner.GetOpNonce()] = true
		}
	}
	return rejected
}

func filterResolvedOperations(
	ops []*SOOperation,
	accepted map[string]uint64,
	rejected map[string]map[uint64]bool,
) []*SOOperation {
	return slices.DeleteFunc(ops, func(op *SOOperation) bool {
		if op == nil {
			return false
		}
		inner := &SOOperationInner{}
		if err := inner.UnmarshalVT(op.GetInner()); err != nil {
			return false
		}
		peerID := inner.GetPeerId()
		nonce := inner.GetNonce()
		if limit, ok := accepted[peerID]; ok && nonce <= limit {
			return true
		}
		return rejected[peerID][nonce]
	})
}

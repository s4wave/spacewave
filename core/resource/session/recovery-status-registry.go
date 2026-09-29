package resource_session

import (
	"sync"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/session"
	s4wave_status "github.com/s4wave/spacewave/sdk/status"
)

// RecoveryStatusRegistry owns volatile renderer-published recovery facts for
// logical sessions. Multiple mounted SessionResources for the same SessionRef
// share one status container through this registry.
type RecoveryStatusRegistry struct {
	mtx       sync.Mutex
	bySession map[string]*ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest]
}

// NewRecoveryStatusRegistry creates a new recovery status registry.
func NewRecoveryStatusRegistry() *RecoveryStatusRegistry {
	return &RecoveryStatusRegistry{bySession: make(map[string]*ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest])}
}

// GetSessionRecoveryStatusCtr returns the shared volatile recovery status
// container for sess.
func (r *RecoveryStatusRegistry) GetSessionRecoveryStatusCtr(
	sess session.Session,
) *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	if r == nil || sess == nil || sess.GetSessionRef() == nil {
		return newRendererRecoveryCtr()
	}
	return r.getSessionRecoveryStatusCtrForRef(sess.GetSessionRef())
}

// getSessionRecoveryStatusCtrForRef shares renderer status by the complete provider resource identity.
func (r *RecoveryStatusRegistry) getSessionRecoveryStatusCtrForRef(
	ref *session.SessionRef,
) *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	// Return a detached container for a nil registry or reference.
	if r == nil || ref == nil {
		return newRendererRecoveryCtr()
	}

	// Reuse or create the session's shared status container.
	key := recoveryStatusSessionKey(ref)
	r.mtx.Lock()
	defer r.mtx.Unlock()
	ctr := r.bySession[key]
	if ctr == nil {
		ctr = newRendererRecoveryCtr()
		r.bySession[key] = ctr
	}
	return ctr
}

// recoveryStatusSessionKey addresses recovery status by provider, account, and session.
func recoveryStatusSessionKey(ref *session.SessionRef) string {
	providerRef := ref.GetProviderResourceRef()
	return providerRef.GetProviderId() + "/" + providerRef.GetProviderAccountId() + "/" + providerRef.GetId()
}

// newRendererRecoveryCtr constructs a volatile container that notifies only on changed reports.
func newRendererRecoveryCtr() *ccontainer.CContainer[*s4wave_status.ReportRecoveryStatusRequest] {
	return ccontainer.NewCContainerWithEqual(nil, rendererRecoveryStatusEqual)
}

package resource_server

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

type pendingResourceWarning struct {
	clientID         uint32
	parentResourceID uint32
	resourceID       uint32
	age              time.Duration
	serviceID        string
	methodID         string
}

// scanPendingResources reports each pending resource once after its warning age.
func (s *ResourceServer) scanPendingResources(ctx context.Context, client *RemoteResourceClient) {
	warned := make(map[uint32]struct{})
	for {
		// Snapshot warnings and the next state transition under the lifecycle lock.
		var warnings []pendingResourceWarning
		var waitCh <-chan struct{}
		var nextWarning time.Duration
		var released bool
		now := s.now()
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			if client.released {
				released = true
				return
			}
			for id, res := range client.resources {
				if !res.pending {
					continue
				}
				if _, ok := warned[id]; ok {
					continue
				}
				age := now.Sub(res.createdAt)
				if age >= s.pendingWarningAge {
					warned[id] = struct{}{}
					warnings = append(warnings, pendingResourceWarning{
						clientID:         client.clientID,
						parentResourceID: res.parentResourceID,
						resourceID:       id,
						age:              age,
						serviceID:        res.serviceID,
						methodID:         res.methodID,
					})
					continue
				}
				remaining := s.pendingWarningAge - age
				if nextWarning == 0 || remaining < nextWarning {
					nextWarning = remaining
				}
			}
		})
		if released {
			return
		}

		// Emit diagnostics without holding the lifecycle lock.
		for _, warning := range warnings {
			s.warnPendingResource(warning)
		}
		if len(warnings) != 0 {
			continue
		}

		// Wait for resource state to change or for the oldest pending resource.
		var timer *time.Timer
		var timerCh <-chan time.Time
		if nextWarning != 0 {
			timer = time.NewTimer(nextWarning)
			timerCh = timer.C
		}
		select {
		case <-ctx.Done():
		case <-waitCh:
		case <-timerCh:
		}
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *ResourceServer) warnPendingResource(warning pendingResourceWarning) {
	if s.pendingWarningHandler != nil {
		s.pendingWarningHandler(warning)
		return
	}
	s.le.WithFields(logrus.Fields{
		"client_id":          warning.clientID,
		"parent_resource_id": warning.parentResourceID,
		"resource_id":        warning.resourceID,
		"age":                warning.age,
		"service_id":         warning.serviceID,
		"method_id":          warning.methodID,
	}).Warn("pending resource was not adopted")
}

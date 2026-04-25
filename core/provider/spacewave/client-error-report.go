package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

const clientErrorReportCodeSharedObjectInitialStateRejected = "shared_object_initial_state_rejected"

const clientErrorReportComponentSharedObjectTracker = "shared-object-tracker"

// reportClientError submits a best-effort diagnostic report to the cloud.
// Errors are logged and otherwise ignored so local failure handling stays bounded.
func (a *ProviderAccount) reportClientError(
	ctx context.Context,
	errorCode string,
	component string,
	resourceType string,
	resourceID string,
	detail string,
) {
	if errorCode == "" || component == "" {
		return
	}

	reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	cli, _, _, err := a.getReadySessionClient(reportCtx)
	if err != nil {
		a.le.WithError(err).
			WithField("error-code", errorCode).
			WithField("component", component).
			Debug("failed to resolve session client for client error report")
		return
	}
	if err := cli.PostClientErrorReport(
		reportCtx,
		errorCode,
		component,
		resourceType,
		resourceID,
		detail,
	); err != nil {
		var ce *cloudError
		if errors.As(err, &ce) && ce.StatusCode == 404 {
			a.le.WithError(err).
				WithField("error-code", errorCode).
				WithField("component", component).
				Debug("client error report endpoint unavailable")
			return
		}
		a.le.WithError(err).
			WithField("error-code", errorCode).
			WithField("component", component).
			Warn("failed to submit client error report")
	}
}

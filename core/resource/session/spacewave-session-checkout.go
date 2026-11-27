package resource_session

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// CreateCheckoutSession creates or resumes a Stripe Checkout Session.
func (r *SpacewaveSessionResource) CreateCheckoutSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateCheckoutSessionRequest,
) (*s4wave_provider_spacewave.CreateCheckoutSessionResponse, error) {
	// Require both redirect URLs before creating a Checkout Session.
	if req.GetSuccessUrl() == "" || req.GetCancelUrl() == "" {
		return nil, errors.New("success_url and cancel_url required")
	}

	// Create the Cloud Checkout Session and retain its status ticket.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.CreateCheckoutSession(ctx, req)
	if err != nil {
		return nil, err
	}

	// Store the ticket on the ProviderAccount so WatchCheckoutStatus can use it.
	if resp.GetStatus() == "pending" && resp.GetWsTicket() != "" {
		r.swAcc.GetCheckoutWatcher().SetTicket(resp.GetWsTicket())
	}

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.CreateCheckoutSessionResponse{
		CheckoutUrl: resp.GetCheckoutUrl(),
		WsTicket:    resp.GetWsTicket(),
		Status:      mapCheckoutStatus(resp.GetStatus()),
	}, nil
}

// CancelCheckoutSession cancels pending checkout attempts.
func (r *SpacewaveSessionResource) CancelCheckoutSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.CancelCheckoutSessionRequest,
) (*s4wave_provider_spacewave.CancelCheckoutSessionResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	resp, err := cli.CancelCheckoutSession(ctx)
	if err != nil {
		return nil, err
	}

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.CancelCheckoutSessionResponse{
		Status: mapCheckoutStatus(resp.GetStatus()),
	}, nil
}

// WatchCheckoutStatus streams checkout status changes via the checkout WS.
func (r *SpacewaveSessionResource) WatchCheckoutStatus(
	req *s4wave_provider_spacewave.WatchCheckoutStatusRequest,
	strm s4wave_session.SRPCSpacewaveSessionResourceService_WatchCheckoutStatusStream,
) error {
	// Scope the checkout watch to the caller's stream.
	ctx := strm.Context()

	// Retain the shared checkout watcher for this stream.
	watcher := r.swAcc.GetCheckoutWatcher()
	ref := watcher.AddRef()
	defer ref.Release()

	// Remember the last checkout status sent to this caller.
	var prev s4wave_provider_spacewave.CheckoutStatus
	for {
		ch, status := watcher.WaitStatus()

		mapped := mapCheckoutStatus(status)
		if mapped != prev {
			if err := strm.Send(&s4wave_provider_spacewave.WatchCheckoutStatusResponse{
				Status: mapped,
			}); err != nil {
				return err
			}
			prev = mapped
		}

		// Terminal statuses end the stream.
		if status == "completed" || status == "expired" {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// mapCheckoutStatus maps a checkout status string to the CheckoutStatus enum.
func mapCheckoutStatus(s string) s4wave_provider_spacewave.CheckoutStatus {
	switch s {
	case "pending":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_PENDING
	case "completed":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_COMPLETED
	case "expired":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_EXPIRED
	case "canceled":
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_CANCELED
	default:
		return s4wave_provider_spacewave.CheckoutStatus_CheckoutStatus_UNKNOWN
	}
}

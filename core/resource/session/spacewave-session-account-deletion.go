package resource_session

import (
	"context"

	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// RequestDeleteNowEmail sends a delete-now confirmation email with a code and link.
func (r *SpacewaveSessionResource) RequestDeleteNowEmail(
	ctx context.Context,
	_ *s4wave_provider_spacewave.RequestDeleteNowEmailRequest,
) (*s4wave_provider_spacewave.RequestDeleteNowEmailResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	result, err := cli.RequestDeleteNowEmail(ctx)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RequestDeleteNowEmailResponse{
		Sent:       true,
		RetryAfter: result.RetryAfter,
		Email:      result.Email,
	}, nil
}

// ConfirmDeleteNowCode finalizes delete-now using the 6-digit code from email.
func (r *SpacewaveSessionResource) ConfirmDeleteNowCode(
	ctx context.Context,
	req *s4wave_provider_spacewave.ConfirmDeleteNowCodeRequest,
) (*s4wave_provider_spacewave.ConfirmDeleteNowCodeResponse, error) {
	// Confirm the emailed deletion code and invalidate cached account state.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	result, err := cli.ConfirmDeleteNowCode(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.ConfirmDeleteNowCodeResponse{
		DeleteAt:         result.DeleteAt,
		InvoiceTotal:     result.InvoiceTotal,
		InvoiceAmountDue: result.InvoiceAmountDue,
		InvoiceCurrency:  result.InvoiceCurrency,
		InvoiceStatus:    result.InvoiceStatus,
		ChargeAttempted:  result.ChargeAttempted,
		RefundAmount:     result.RefundAmount,
		RefundCurrency:   result.RefundCurrency,
	}, nil
}

// UndoDeleteNow cancels a pending delete-now countdown.
func (r *SpacewaveSessionResource) UndoDeleteNow(
	ctx context.Context,
	_ *s4wave_provider_spacewave.UndoDeleteNowRequest,
) (*s4wave_provider_spacewave.UndoDeleteNowResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	if err := cli.UndoDeleteNow(ctx); err != nil {
		return nil, err
	}
	r.swAcc.BumpLocalEpoch()
	return &s4wave_provider_spacewave.UndoDeleteNowResponse{}, nil
}

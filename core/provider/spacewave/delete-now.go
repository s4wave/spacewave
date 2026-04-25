package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// RequestDeleteNowEmailResult is the parsed response from a delete-now email request.
type RequestDeleteNowEmailResult struct {
	// RetryAfter is the resend cooldown in seconds.
	RetryAfter uint32
	// Email is the verified address that received the delete-now email.
	Email string
}

// ConfirmDeleteNowCodeResult is the parsed response from delete-now confirmation.
type ConfirmDeleteNowCodeResult struct {
	// DeleteAt is when the pending-delete countdown ends.
	DeleteAt uint64
	// InvoiceTotal is Stripe's final invoice total in cents.
	InvoiceTotal int64
	// InvoiceAmountDue is the final invoice amount due in cents.
	InvoiceAmountDue int64
	// InvoiceCurrency is the final invoice currency code.
	InvoiceCurrency string
	// InvoiceStatus is the final invoice status.
	InvoiceStatus string
	// ChargeAttempted reports whether Stripe attempted an immediate charge.
	ChargeAttempted bool
	// RefundAmount is the immediate refund amount in cents.
	RefundAmount int64
	// RefundCurrency is the refund currency code.
	RefundCurrency string
}

// RequestDeleteNowEmail sends a delete-now confirmation email with a code and link.
func (c *SessionClient) RequestDeleteNowEmail(ctx context.Context) (*RequestDeleteNowEmailResult, error) {
	body, err := (&api.RequestDeleteNowEmailRequest{}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal delete-now request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/delete/request", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "request delete-now email")
	}
	var resp api.RequestDeleteNowEmailResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal delete-now request response")
	}
	return &RequestDeleteNowEmailResult{
		RetryAfter: resp.GetRetryAfter(),
		Email:      resp.GetEmail(),
	}, nil
}

// ConfirmDeleteNowCode finalizes delete-now using the 6-digit email code.
func (c *SessionClient) ConfirmDeleteNowCode(ctx context.Context, code string) (*ConfirmDeleteNowCodeResult, error) {
	body, err := (&api.DeleteNowVerifyCodeRequest{Code: code}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal delete-now verify request")
	}
	data, err := c.doPostBinary(ctx, "/api/account/delete/verify-code", body, nil, SeedReasonMutation)
	if err != nil {
		return nil, errors.Wrap(err, "confirm delete-now code")
	}
	var resp api.DeleteNowVerifyCodeResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal delete-now confirm response")
	}
	return &ConfirmDeleteNowCodeResult{
		DeleteAt:         uint64(resp.GetDeleteAt()),
		InvoiceTotal:     resp.GetInvoiceTotal(),
		InvoiceAmountDue: resp.GetInvoiceAmountDue(),
		InvoiceCurrency:  resp.GetInvoiceCurrency(),
		InvoiceStatus:    resp.GetInvoiceStatus(),
		ChargeAttempted:  resp.GetChargeAttempted(),
		RefundAmount:     resp.GetRefundAmount(),
		RefundCurrency:   resp.GetRefundCurrency(),
	}, nil
}

// UndoDeleteNow cancels a pending delete-now countdown.
func (c *SessionClient) UndoDeleteNow(ctx context.Context) error {
	body, err := (&api.UndoDeleteRequest{}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal undo-delete request")
	}
	_, err = c.doPostBinary(ctx, "/api/account/undo-delete", body, nil, SeedReasonMutation)
	return errors.Wrap(err, "undo delete-now")
}

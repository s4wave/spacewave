package s4wave_provider_spacewave

// BillingStatusFromString parses a subscription status string (as returned by
// the Stripe API / spacewave-cloud) into the BillingStatus enum.
func BillingStatusFromString(s string) BillingStatus {
	switch s {
	case "none":
		return BillingStatus_BillingStatus_NONE
	case "active":
		return BillingStatus_BillingStatus_ACTIVE
	case "trialing":
		return BillingStatus_BillingStatus_TRIALING
	case "past_due":
		return BillingStatus_BillingStatus_PAST_DUE
	case "past_due_readonly":
		return BillingStatus_BillingStatus_PAST_DUE_READONLY
	case "canceled":
		return BillingStatus_BillingStatus_CANCELED
	case "deleted":
		return BillingStatus_BillingStatus_DELETED
	case "lapsed":
		return BillingStatus_BillingStatus_LAPSED
	default:
		return BillingStatus_BillingStatus_UNKNOWN
	}
}

// NormalizedString returns the normalized string form used by cloud internals.
func (s BillingStatus) NormalizedString() string {
	switch s {
	case BillingStatus_BillingStatus_NONE:
		return "none"
	case BillingStatus_BillingStatus_ACTIVE:
		return "active"
	case BillingStatus_BillingStatus_TRIALING:
		return "trialing"
	case BillingStatus_BillingStatus_PAST_DUE:
		return "past_due"
	case BillingStatus_BillingStatus_PAST_DUE_READONLY:
		return "past_due_readonly"
	case BillingStatus_BillingStatus_CANCELED:
		return "canceled"
	case BillingStatus_BillingStatus_DELETED:
		return "deleted"
	case BillingStatus_BillingStatus_LAPSED:
		return "lapsed"
	default:
		return ""
	}
}

// IsWriteAllowed returns true if the billing status permits data writes.
// Statuses that allow writes: ACTIVE, TRIALING, PAST_DUE. Past-due accounts
// retain write access until the grace period expires and the status transitions
// to PAST_DUE_READONLY.
func (s BillingStatus) IsWriteAllowed() bool {
	switch s {
	case BillingStatus_BillingStatus_ACTIVE,
		BillingStatus_BillingStatus_TRIALING,
		BillingStatus_BillingStatus_PAST_DUE:
		return true
	default:
		return false
	}
}

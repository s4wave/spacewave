package sessioninfo

import (
	"github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ShouldEmitOnboardingStatus returns whether the Onboarding Status projection
// should be sent given the current account state.
func ShouldEmitOnboardingStatus(stateLoaded bool, accountStatus provider.ProviderAccountStatus) bool {
	if stateLoaded {
		return true
	}
	switch accountStatus {
	case provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED,
		provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
		provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT,
		provider.ProviderAccountStatus_ProviderAccountStatus_FAILED:
		return true
	}
	return false
}

// BuildEmptyBillingUsageInfo returns a BillingUsageInfo with only the current
// offer's terms.
func BuildEmptyBillingUsageInfo() *s4wave_provider_spacewave.BillingUsageInfo {
	offer := api.CurrentCloudOffer()
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBaselineBytes:           float64(offer.StorageBytes),
		OfferVersion:                   offer.Version,
		MonthlyPriceCents:              offer.MonthlyPriceCents,
		StorageMicrodollarsPerGibMonth: offer.StorageMicrodollarsPerGibMonth,
		PolicyVersion:                  offer.PolicyVersion,
	}
}

// BuildBillingUsageInfo projects the payer's accepted usage, period, and budget.
func BuildBillingUsageInfo(usage *api.BillingUsageResponse) *s4wave_provider_spacewave.BillingUsageInfo {
	if usage == nil {
		return BuildEmptyBillingUsageInfo()
	}
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBytes:                   usage.GetStorageBytes(),
		StorageBaselineBytes:           usage.GetStorageBaselineBytes(),
		WriteOps:                       usage.GetWriteOps(),
		ReadOps:                        usage.GetReadOps(),
		UsageMeteredThroughAt:          usage.GetUsageMeteredThroughAt(),
		OverageLimitCents:              usage.GetOverageLimitCents(),
		AccruedOverageMicrodollars:     usage.GetAccruedOverageMicrodollars(),
		CurrentPeriodStart:             usage.GetCurrentPeriodStart(),
		CurrentPeriodEnd:               usage.GetCurrentPeriodEnd(),
		OfferVersion:                   usage.GetOfferVersion(),
		MonthlyPriceCents:              usage.GetMonthlyPriceCents(),
		StorageMicrodollarsPerGibMonth: usage.GetStorageMicrodollarsPerGibMonth(),
		PolicyVersion:                  usage.GetPolicyVersion(),
	}
}

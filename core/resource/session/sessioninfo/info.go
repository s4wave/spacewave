package sessioninfo

import (
	"math"

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

// BuildEmptyBillingUsageInfo returns a BillingUsageInfo with only baseline values.
func BuildEmptyBillingUsageInfo() *s4wave_provider_spacewave.BillingUsageInfo {
	offer := api.CurrentCloudOffer()
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBaselineBytes: float64(offer.StorageBytes),
		WriteOpsBaseline:     cloudOfferOperationsInt64(offer.WriteOperations),
		ReadOpsBaseline:      cloudOfferOperationsInt64(offer.ReadOperations),
		OfferVersion:         offer.Version,
		MonthlyPriceCents:    offer.MonthlyPriceCents,
		WriteMicrodollars:    offer.WriteMicrodollars,
		ReadMicrodollars:     offer.ReadMicrodollars,
		PolicyVersion:        offer.PolicyVersion,
	}
}

func cloudOfferOperationsInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		panic("cloud offer operation baseline exceeds int64 range")
	}
	return int64(value) //nolint:gosec // the explicit MaxInt64 check protects the billing projection field.
}

// BuildBillingUsageInfo projects the payer's accepted usage, period, and budget.
func BuildBillingUsageInfo(usage *api.BillingUsageResponse) *s4wave_provider_spacewave.BillingUsageInfo {
	if usage == nil {
		return BuildEmptyBillingUsageInfo()
	}
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBytes:                usage.GetStorageBytes(),
		StorageBaselineBytes:        usage.GetStorageBaselineBytes(),
		WriteOps:                    usage.GetWriteOps(),
		WriteOpsBaseline:            usage.GetWriteOpsBaseline(),
		ReadOps:                     usage.GetReadOps(),
		ReadOpsBaseline:             usage.GetReadOpsBaseline(),
		UsageMeteredThroughAt:       usage.GetUsageMeteredThroughAt(),
		OverageLimitCents:           usage.GetOverageLimitCents(),
		AccruedOverageMicrodollars:  usage.GetAccruedOverageMicrodollars(),
		CurrentPeriodStart:          usage.GetCurrentPeriodStart(),
		CurrentPeriodEnd:            usage.GetCurrentPeriodEnd(),
		ReservedOverageMicrodollars: usage.GetReservedOverageMicrodollars(),
		OfferVersion:                usage.GetOfferVersion(),
		MonthlyPriceCents:           usage.GetMonthlyPriceCents(),
		WriteMicrodollars:           usage.GetWriteMicrodollars(),
		ReadMicrodollars:            usage.GetReadMicrodollars(),
		PolicyVersion:               usage.GetPolicyVersion(),
	}
}

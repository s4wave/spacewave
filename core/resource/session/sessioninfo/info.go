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

// BuildEmptyBillingUsageInfo returns a BillingUsageInfo with only baseline values.
func BuildEmptyBillingUsageInfo() *s4wave_provider_spacewave.BillingUsageInfo {
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBaselineBytes: storageBaselineBytes,
		WriteOpsBaseline:     writeOpsBaseline,
		ReadOpsBaseline:      readOpsBaseline,
	}
}

// BuildBillingUsageInfo converts a cloud billing usage response to a proto BillingUsageInfo.
func BuildBillingUsageInfo(
	usage *api.BillingUsageResponse,
) *s4wave_provider_spacewave.BillingUsageInfo {
	if usage == nil {
		return BuildEmptyBillingUsageInfo()
	}
	return &s4wave_provider_spacewave.BillingUsageInfo{
		StorageBytes:                             usage.GetStorageBytes(),
		StorageBaselineBytes:                     storageBaselineBytes,
		WriteOps:                                 usage.GetWriteOps(),
		WriteOpsBaseline:                         writeOpsBaseline,
		ReadOps:                                  usage.GetReadOps(),
		ReadOpsBaseline:                          readOpsBaseline,
		StorageOverageBytes:                      usage.GetStorageOverageBytes(),
		StorageOverageMonthlyCostEstimateUsd:     usage.GetStorageOverageMonthlyCostEstimateUsd(),
		StorageOverageMonthToDateGbMonths:        usage.GetStorageOverageMonthToDateGbMonths(),
		StorageOverageMonthToDateCostEstimateUsd: usage.GetStorageOverageMonthToDateCostEstimateUsd(),
		StorageOverageDeletedGbMonths:            usage.GetStorageOverageDeletedGbMonths(),
		StorageOverageDeletedCostEstimateUsd:     usage.GetStorageOverageDeletedCostEstimateUsd(),
		UsageMeteredThroughAt:                    usage.GetUsageMeteredThroughAt(),
	}
}

// storageBaselineBytes is the included storage baseline (100 GB).
const storageBaselineBytes = 100 * 1024 * 1024 * 1024

// writeOpsBaseline is the included write ops baseline per period (1M).
const writeOpsBaseline = 1000000

// readOpsBaseline is the included read ops baseline per period (10M).
const readOpsBaseline = 10000000

//go:build !goscript

package resource_session

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestShouldEmitOnboardingStatusFirstEmissionGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		stateLoaded   bool
		accountStatus provider.ProviderAccountStatus
		expected      bool
	}{
		{
			name:          "holds none without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_NONE,
		},
		{
			name:          "holds pending without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_PENDING,
		},
		{
			name:          "holds ready without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
		},
		{
			name:          "emits unauthenticated without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED,
			expected:      true,
		},
		{
			name:          "emits deleted without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_DELETED,
			expected:      true,
		},
		{
			name:          "emits dormant without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT,
			expected:      true,
		},
		{
			name:          "emits failed without account state",
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_FAILED,
			expected:      true,
		},
		{
			name:          "emits once account state loaded",
			stateLoaded:   true,
			accountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_PENDING,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldEmitOnboardingStatus(tt.stateLoaded, tt.accountStatus)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestBuildBillingUsageInfoIncludesStorageOverageFields(t *testing.T) {
	usage := buildBillingUsageInfo(&api.BillingUsageResponse{
		StorageBytes:                             123,
		WriteOps:                                 4,
		ReadOps:                                  5,
		StorageOverageBytes:                      23,
		StorageOverageMonthlyCostEstimateUsd:     0.46,
		StorageOverageMonthToDateGbMonths:        1.25,
		StorageOverageMonthToDateCostEstimateUsd: 0.025,
		StorageOverageDeletedGbMonths:            0.5,
		StorageOverageDeletedCostEstimateUsd:     0.01,
		UsageMeteredThroughAt:                    1776900000000,
	})

	if usage.GetStorageOverageBytes() != 23 {
		t.Fatalf("expected current storage overage bytes, got %+v", usage)
	}
	if usage.GetStorageOverageMonthlyCostEstimateUsd() != 0.46 {
		t.Fatalf("expected monthly cost estimate, got %+v", usage)
	}
	if usage.GetStorageOverageMonthToDateGbMonths() != 1.25 {
		t.Fatalf("expected month-to-date GB-months, got %+v", usage)
	}
	if usage.GetStorageOverageMonthToDateCostEstimateUsd() != 0.025 {
		t.Fatalf("expected month-to-date cost, got %+v", usage)
	}
	if usage.GetStorageOverageDeletedGbMonths() != 0.5 {
		t.Fatalf("expected deleted-data GB-months, got %+v", usage)
	}
	if usage.GetStorageOverageDeletedCostEstimateUsd() != 0.01 {
		t.Fatalf("expected deleted-data cost, got %+v", usage)
	}
	if usage.GetUsageMeteredThroughAt() != 1776900000000 {
		t.Fatalf("expected usage freshness timestamp, got %+v", usage)
	}
}

package resource_session

import (
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestShouldLoadManagedBillingSummaryReady(t *testing.T) {
	if !shouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_READY) {
		t.Fatal("expected READY onboarding status to load billing summary")
	}
}

func TestShouldLoadManagedBillingSummaryUnauthenticated(t *testing.T) {
	if shouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED) {
		t.Fatal("expected UNAUTHENTICATED onboarding status to skip billing summary")
	}
}

func TestShouldLoadManagedBillingSummaryDormant(t *testing.T) {
	if shouldLoadManagedBillingSummary(provider.ProviderAccountStatus_ProviderAccountStatus_DORMANT) {
		t.Fatal("expected DORMANT onboarding status to skip billing summary")
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

package provider_spacewave

import "testing"

func TestCdnRootChangedCallbackReleaseDuringDeliveryKeepsSnapshot(t *testing.T) {
	acc := &ProviderAccount{}
	secondCalls := 0
	var releaseSecond func()
	releaseFirst := acc.RegisterCdnRootChangedCallback(func(string) {
		releaseSecond()
	})
	releaseSecond = acc.RegisterCdnRootChangedCallback(func(string) {
		secondCalls++
	})

	acc.fireCdnRootChanged("space-1")
	if secondCalls != 1 {
		t.Fatalf("second callback calls = %d, want 1 from delivery snapshot", secondCalls)
	}

	acc.fireCdnRootChanged("space-1")
	if secondCalls != 1 {
		t.Fatalf("released callback was called again: %d", secondCalls)
	}
	releaseFirst()
}

func TestCdnRootChangedCallbackReentryRegistrationStartsNextEvent(t *testing.T) {
	acc := &ProviderAccount{}
	lateCalls := 0
	releaseFirst := acc.RegisterCdnRootChangedCallback(func(string) {
		acc.RegisterCdnRootChangedCallback(func(string) {
			lateCalls++
		})
	})

	acc.fireCdnRootChanged("space-1")
	if lateCalls != 0 {
		t.Fatalf("reentrant registration joined current delivery: %d", lateCalls)
	}

	acc.fireCdnRootChanged("space-1")
	if lateCalls != 1 {
		t.Fatalf("late callback calls = %d, want 1 on next event", lateCalls)
	}
	releaseFirst()
}

func TestCdnRootChangedCallbackReleaseSelfDuringDelivery(t *testing.T) {
	acc := &ProviderAccount{}
	calls := 0
	var release func()
	release = acc.RegisterCdnRootChangedCallback(func(string) {
		calls++
		release()
	})

	acc.fireCdnRootChanged("space-1")
	acc.fireCdnRootChanged("space-1")
	if calls != 1 {
		t.Fatalf("self-released callback calls = %d, want 1", calls)
	}
}

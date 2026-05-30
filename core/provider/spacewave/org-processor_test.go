package provider_spacewave

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

type recordingOrgProcessorKeySet struct {
	mtx   sync.Mutex
	syncs [][]string
}

func (r *recordingOrgProcessorKeySet) SyncKeys(keys []string, _ bool) ([]string, []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.syncs = append(r.syncs, slices.Clone(keys))
	return nil, nil
}

func (r *recordingOrgProcessorKeySet) snapshot() [][]string {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	out := make([][]string, 0, len(r.syncs))
	for _, keys := range r.syncs {
		out = append(out, slices.Clone(keys))
	}
	return out
}

func TestOrgProcessorDesiredKeysSyncsOnlyOnDesiredKeyChange(t *testing.T) {
	acc := &ProviderAccount{}
	rec := &recordingOrgProcessorKeySet{}
	desired := newOrgProcessorDesiredKeys(acc, rec, orgProcessorTestSOList("org-1"))

	desired.sync()
	if got := rec.snapshot(); len(got) != 0 {
		t.Fatalf("sync before valid org list = %v, want none", got)
	}

	setOrgProcessorTestOrgList(acc, []*api.OrgResponse{{Id: "org-1", Role: "org:owner"}})
	desired.sync()
	assertOrgProcessorSyncs(t, rec.snapshot(), [][]string{{"org-1"}})

	desired.sync()
	assertOrgProcessorSyncs(t, rec.snapshot(), [][]string{{"org-1"}})

	setOrgProcessorTestOrgList(acc, []*api.OrgResponse{{Id: "org-1", Role: "org:member"}})
	desired.sync()
	assertOrgProcessorSyncs(t, rec.snapshot(), [][]string{{"org-1"}, nil})
}

func TestOrgProcessorDesiredKeysSOListChangeFeedsProcessors(t *testing.T) {
	acc := &ProviderAccount{}
	setOrgProcessorTestOrgList(acc, []*api.OrgResponse{
		{Id: "org-1", Role: "org:owner"},
		{Id: "org-2", Role: "owner"},
	})
	rec := &recordingOrgProcessorKeySet{}
	desired := newOrgProcessorDesiredKeys(acc, rec, orgProcessorTestSOList("org-1"))

	desired.sync()
	desired.setSOList(orgProcessorTestSOList("org-1", "org-2"))

	assertOrgProcessorSyncs(t, rec.snapshot(), [][]string{
		{"org-1"},
		{"org-1", "org-2"},
	})
}

func TestOrgProcessorDesiredKeysWatchSOListFeedsProcessors(t *testing.T) {
	initial := orgProcessorTestSOList("org-1")
	acc := &ProviderAccount{
		soListCtr: ccontainer.NewCContainer(initial),
	}
	setOrgProcessorTestOrgList(acc, []*api.OrgResponse{
		{Id: "org-1", Role: "org:owner"},
		{Id: "org-2", Role: "org:owner"},
	})
	rec := &recordingOrgProcessorKeySet{}
	desired := newOrgProcessorDesiredKeys(acc, rec, initial)
	desired.sync()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- desired.watchSOList(ctx)
	}()

	acc.soListCtr.SetValue(orgProcessorTestSOList("org-1", "org-2"))
	waitForOrgProcessorSync(t, rec, []string{"org-1", "org-2"})

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("watchSOList error = %v, want context.Canceled", err)
	}
}

func TestOrgProcessorDesiredKeysWatchOrgListFeedsProcessors(t *testing.T) {
	acc := &ProviderAccount{}
	rec := &recordingOrgProcessorKeySet{}
	desired := newOrgProcessorDesiredKeys(acc, rec, orgProcessorTestSOList("org-1"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- desired.watchOrgList(ctx)
	}()

	deadline := time.After(time.Second)
	for !hasOrgProcessorSync(rec, []string{"org-1"}) {
		setOrgProcessorTestOrgList(acc, []*api.OrgResponse{{Id: "org-1", Role: "owner"}})
		select {
		case <-time.After(time.Millisecond):
		case <-deadline:
			t.Fatal("timed out waiting for org-list watcher sync")
		}
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("watchOrgList error = %v, want context.Canceled", err)
	}
}

func TestRunOrgProcessorDesiredKeysInitialSyncBeforeWatcherStart(t *testing.T) {
	initial := orgProcessorTestSOList("org-1")
	acc := &ProviderAccount{
		soListCtr: ccontainer.NewCContainer(initial),
	}
	setOrgProcessorTestOrgList(acc, []*api.OrgResponse{
		{Id: "org-1", Role: "org:owner"},
		{Id: "org-2", Role: "org:owner"},
	})
	rec := &recordingOrgProcessorKeySet{}
	desired := newOrgProcessorDesiredKeys(acc, rec, initial)
	ctx, cancel := context.WithCancel(context.Background())
	initialSynced := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- acc.runOrgProcessorDesiredKeysWithHooks(ctx, desired, orgProcessorHooks{
			afterInitialSync: func() {
				close(initialSynced)
				acc.soListCtr.SetValue(orgProcessorTestSOList("org-1", "org-2"))
			},
		})
	}()
	<-initialSynced
	waitForOrgProcessorSync(t, rec, []string{"org-1", "org-2"})
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runOrgProcessorDesiredKeysWithHooks error = %v, want context.Canceled", err)
	}

	assertOrgProcessorSyncs(t, rec.snapshot(), [][]string{
		{"org-1"},
		{"org-1", "org-2"},
	})
}

func setOrgProcessorTestOrgList(acc *ProviderAccount, orgs []*api.OrgResponse) {
	acc.orgBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		acc.orgList = orgs
		acc.orgListValid = true
		broadcast()
	})
}

func orgProcessorTestSOList(ids ...string) *sobject.SharedObjectList {
	entries := make([]*sobject.SharedObjectListEntry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, &sobject.SharedObjectListEntry{
			Ref:  sobject.NewSharedObjectRef("spacewave", "acct-1", id, SobjectBlockStoreID(id)),
			Meta: &sobject.SharedObjectMeta{BodyType: s4wave_org.OrgBodyType},
		})
	}
	return &sobject.SharedObjectList{SharedObjects: entries}
}

func assertOrgProcessorSyncs(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sync count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Fatalf("sync[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func waitForOrgProcessorSync(t *testing.T, rec *recordingOrgProcessorKeySet, want []string) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if hasOrgProcessorSync(rec, want) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sync %v; got %v", want, rec.snapshot())
		case <-tick.C:
		}
	}
}

func hasOrgProcessorSync(rec *recordingOrgProcessorKeySet, want []string) bool {
	for _, got := range rec.snapshot() {
		if slices.Equal(got, want) {
			return true
		}
	}
	return false
}

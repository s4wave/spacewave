//go:build !tinygo

package block_packsweep_admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// leafOpts declares a block with no children.
var leafOpts = &block.PutOpts{RefsKnown: true}

// fixedRefs serves the same child list for every block.
type fixedRefs []*block.BlockRef

// BlockRefs answers every block with the list.
func (f fixedRefs) BlockRefs(_ context.Context, refs []*block.BlockRef) ([]packsweep.RefsResult, error) {
	out := make([]packsweep.RefsResult, len(refs))
	for i := range out {
		out[i] = packsweep.RefsResult{Refs: f}
	}
	return out, nil
}

// testBackend is a pack store over an in-memory bucket, a write log that is
// on, and the service over them, reached through an in-memory pipe.
type testBackend struct {
	bucket *s3test.Bucket
	client *block_store_s3.Client
	store  *block_store_s3.PackStore
	log    *writelog.Log
	on     atomic.Bool
	server *Server
	admin  *Client
}

// newTestBackend opens a backend with the switch off.
func newTestBackend(t *testing.T) *testBackend {
	t.Helper()
	b := &testBackend{}
	b.bucket, b.client = s3test.New(t)
	b.store = block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), b.client, s3test.BucketName, "p/", &block_store_s3.PackStoreOptions{
		KeepPacks:  true,
		Index:      hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()),
		SoleWriter: true,
	})
	t.Cleanup(b.store.Close)
	b.log = writelog.New(logrus.NewEntry(logrus.New()))
	if err := b.log.Set(&writelog.Options{Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.log.Close() })
	b.server = NewServer(Options{Packs: b.store, Log: b.log, Refs: fixedRefs{}, Enabled: b.on.Load})
	mux := srpc.NewMux()
	if err := SRPCRegisterPackSweepAdmin(mux, b.server); err != nil {
		t.Fatal(err)
	}
	b.admin = NewClient(NewSRPCPackSweepAdminClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))))
	return b
}

// put writes a block to its own pack and returns the block and the pack.
func (b *testBackend) put(t *testing.T, data string) (*block.BlockRef, string) {
	t.Helper()
	ref, _, err := b.store.PutBlock(t.Context(), []byte(data), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := b.store.LocatePacks(t.Context(), []*block.BlockRef{ref})
	if err != nil || ids[0] == "" {
		t.Fatalf("locate the put block = %v, %v", ids, err)
	}
	return ref, ids[0]
}

// TestAdminMoves quarantines and lifts a pack through the service: refused
// while the switch is off, idempotent while it is on, and reported by Locate
// and Status after each move.
func TestAdminMoves(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	ref, id := b.put(t, "block")
	pack := &PacksRequest{PackIds: []string{id}}

	// While the switch is off, every move is refused and the reads answer.
	if _, err := b.admin.Quarantine(ctx, pack); err == nil || !strings.Contains(err.Error(), ErrDisabled.Error()) {
		t.Fatalf("quarantine while off = %v", err)
	}
	if _, err := b.admin.Lift(ctx, pack); err == nil {
		t.Fatal("lifted while off")
	}
	if _, err := b.admin.Adopt(ctx, &AdoptRequest{}); err == nil {
		t.Fatal("adopted while off")
	}
	status, err := b.admin.Status(ctx, &StatusRequest{})
	if err != nil || status.GetEnabled() || len(status.GetPacks()) != 0 || !status.GetLog().GetOn() {
		t.Fatalf("status while off = %v, %v", status, err)
	}

	// With the switch on, a quarantine twice holds the pack out of Locate.
	b.on.Store(true)
	for range 2 {
		resp, err := b.admin.Quarantine(ctx, pack)
		if err != nil || len(resp.GetPacks()) != 1 || resp.GetPacks()[0].GetState() != PackState_PACK_STATE_QUARANTINED {
			t.Fatalf("quarantine = %v, %v", resp, err)
		}
	}
	located, err := b.admin.Locate(ctx, &LocateRequest{Refs: []*block.BlockRef{ref}})
	if err != nil || len(located.GetPackIds()) != 1 || located.GetPackIds()[0] != "" {
		t.Fatalf("locate a quarantined block = %v, %v", located, err)
	}
	status, err = b.admin.Status(ctx, &StatusRequest{})
	if err != nil || !status.GetEnabled() || len(status.GetPacks()) != 1 || status.GetPacks()[0].GetPackId() != id {
		t.Fatalf("status = %v, %v", status, err)
	}

	// A lift returns it, with the lift recorded.
	resp, err := b.admin.Lift(ctx, pack)
	if err != nil || len(resp.GetPacks()) != 1 {
		t.Fatalf("lift = %v, %v", resp, err)
	}
	if hold := resp.GetPacks()[0]; hold.GetState() != PackState_PACK_STATE_REGISTERED || hold.GetEventUnixNano() == 0 || len(hold.GetKey()) != 0 {
		t.Fatalf("lift record = %v", hold)
	}
	located, err = b.admin.Locate(ctx, &LocateRequest{Refs: []*block.BlockRef{ref}})
	if err != nil || located.GetPackIds()[0] != id {
		t.Fatalf("locate a lifted block = %v, %v", located, err)
	}

	// A pack the store lacks fails the move, and the batches are bounded.
	if _, err := b.admin.Quarantine(ctx, &PacksRequest{PackIds: []string{"nope"}}); err == nil {
		t.Fatal("quarantined a pack the store lacks")
	}
	if _, err := b.admin.Quarantine(ctx, &PacksRequest{}); err == nil {
		t.Fatal("quarantined no pack")
	}
	if _, err := b.admin.Locate(ctx, &LocateRequest{Refs: make([]*block.BlockRef, MaxRefs+1)}); err == nil {
		t.Fatal("located more than MaxRefs blocks")
	}
}

// TestAdminAdopt registers a pack another writer uploaded, twice, and finds
// its blocks placed.
func TestAdminAdopt(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	b.on.Store(true)
	_, own := b.put(t, "own")

	// Another writer uploads a pack the backend has not heard of.
	node := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), b.client, s3test.BucketName, "p/", nil)
	defer node.Close()
	ref, _, err := node.PutBlock(ctx, []byte("node"), leafOpts)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := node.Packs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var entry *packfile.PackfileEntry
	for _, e := range entries {
		if e.GetId() != own {
			entry = e
		}
	}
	if entry == nil {
		t.Fatal("the node listed no pack of its own")
	}
	located, err := b.admin.Locate(ctx, &LocateRequest{Refs: []*block.BlockRef{ref}})
	if err != nil || located.GetPackIds()[0] != "" {
		t.Fatalf("located a pack before adopting it: %v, %v", located, err)
	}

	// Adopting it twice places its block in it.
	req := &AdoptRequest{Packs: []*AdoptPack{{Entry: entry, Keys: [][]byte{packfile.BlockKey(ref.GetHash())}}}}
	for range 2 {
		if _, err := b.admin.Adopt(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	located, err = b.admin.Locate(ctx, &LocateRequest{Refs: []*block.BlockRef{ref}})
	if err != nil || located.GetPackIds()[0] != entry.GetId() {
		t.Fatalf("locate an adopted block = %v, %v; want %s", located, err, entry.GetId())
	}
}

// TestAdminLogAndRefs reads the write log through the service in pages, trims
// it while the switch is off, and reads the refs of the backend's blocks.
func TestAdminLogAndRefs(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	var refs []*block.BlockRef
	for _, data := range []string{"a", "b", "c"} {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		b.log.Put(ref.GetHash())
		refs = append(refs, ref)
	}
	at, ok := b.log.Position()
	if !ok {
		t.Fatal("the log has no position")
	}

	// A bounded read stops with more to read, and the next one finishes.
	var lines []writelog.Line
	collect := func(l writelog.Line) error { lines = append(lines, l); return nil }
	res, err := b.admin.ReadLog(ctx, writelog.Position{Epoch: at.Epoch}, 2, collect)
	if err != nil || res.State != writelog.ReadMore || len(lines) != 2 {
		t.Fatalf("first read = %+v, %d lines, %v", res, len(lines), err)
	}
	res, err = b.admin.ReadLog(ctx, res.Next, 0, collect)
	if err != nil || res.State != writelog.ReadOpen || len(lines) != 3 {
		t.Fatalf("second read = %+v, %d lines, %v", res, len(lines), err)
	}
	for i, l := range lines {
		if l.Kind != writelog.KindPut || !l.Hash.EqualVT(refs[i].GetHash()) {
			t.Fatalf("line %d = %+v", i, l)
		}
	}

	// A trim runs while the switch is off.
	if err := b.admin.TrimLog(ctx, res.Next); err != nil {
		t.Fatalf("trim while off: %v", err)
	}

	// The refs of the backend's blocks come through in order.
	b.server.opts.Refs = fixedRefs{refs[0]}
	got, err := b.admin.BlockRefs(ctx, refs[1:])
	if err != nil || len(got) != 2 || len(got[1].Refs) != 1 || !got[1].Refs[0].EqualVT(refs[0]) {
		t.Fatalf("block refs = %+v, %v", got, err)
	}
}

// TestAdminHTTP reaches the service through its HTTP handler, behind a check
// of the backend's admin credential that a dial without it fails.
func TestAdminHTTP(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	handler, err := NewHTTPHandler(b.server, "/admin/pack-sweep")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/admin/pack-sweep"

	// A dial without the credential fails.
	if _, _, err := DialHTTP(ctx, url, nil); err == nil {
		t.Fatal("dialed without the credential")
	}

	// With it, the service answers.
	client, closeConn, err := DialHTTP(ctx, url, http.Header{"Authorization": []string{"Bearer admin"}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn()
	status, err := client.Status(ctx, &StatusRequest{})
	if err != nil || status.GetEnabled() || !status.GetLog().GetOn() {
		t.Fatalf("status over HTTP = %v, %v", status, err)
	}
}

// TestAdminBackend checks the client's Backend carries the service's answers
// to a cycle: the states, the log, the clock and the open operations, absent
// when the backend records none.
func TestAdminBackend(t *testing.T) {
	ctx := t.Context()
	b := newTestBackend(t)
	_, id := b.put(t, "block")
	backend := b.admin.Backend()

	// With no operations recorded, Status reports none, and a registered
	// pack with no time.
	before := time.Now()
	st, err := backend.Status(ctx, []string{id})
	if err != nil || len(st.Packs) != 1 || st.Packs[0] != (packsweep.PackHold{ID: id, State: packsweep.PackRegistered}) {
		t.Fatalf("status = %+v, %v", st, err)
	}
	pos, _ := b.log.Position()
	if !st.LogOn || st.LogEpoch != pos.Epoch || st.Operations != nil || st.Now.Before(before) || st.Now.After(time.Now()) {
		t.Fatalf("status = %+v, want the log's epoch %s, no operations and the clock", st, pos.Epoch)
	}

	// The operations carry each start, and zero for none.
	write := time.Unix(0, 12345)
	b.server.opts.Operations = func() (time.Time, time.Time) { return write, time.Time{} }
	if st, err = backend.Status(ctx, []string{id}); err != nil || st.Operations == nil || !st.Operations.Write.Equal(write) || !st.Operations.Read.IsZero() {
		t.Fatalf("status with operations = %+v, %v", st, err)
	}

	// A quarantine and a lift answer each pack's state and time.
	b.on.Store(true)
	holds, err := backend.Quarantine(ctx, []string{id})
	if err != nil || len(holds) != 1 || holds[0].ID != id || holds[0].State != packsweep.PackQuarantined || holds[0].Since.Before(before) {
		t.Fatalf("quarantine = %+v, %v", holds, err)
	}
	if st, err = backend.Status(ctx, []string{id}); err != nil || !st.Packs[0].Since.Equal(holds[0].Since) {
		t.Fatalf("status of the quarantined pack = %+v, %v", st, err)
	}
	if holds, err = backend.Lift(ctx, []string{id}); err != nil || holds[0].State != packsweep.PackRegistered {
		t.Fatalf("lift = %+v, %v", holds, err)
	}

	// The states number as the pack store numbers them.
	for i, want := range []packsweep.PackState{packsweep.PackRegistered, packsweep.PackQuarantined, packsweep.PackTrashing} {
		store := []block_store_s3.PackState{block_store_s3.PackRegistered, block_store_s3.PackQuarantined, block_store_s3.PackTrashing}[i]
		wire := []PackState{PackState_PACK_STATE_REGISTERED, PackState_PACK_STATE_QUARANTINED, PackState_PACK_STATE_TRASHING}[i]
		if packsweep.PackState(store) != want || packsweep.PackState(wire) != want {
			t.Fatalf("state %v numbers as %d in the store and %d on the wire", want, store, wire)
		}
	}
}

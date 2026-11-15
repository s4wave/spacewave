//go:build !js

package sdk_world_engine_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// changeStream is a raw WatchChanges stream with what it received.
type changeStream struct {
	t    *testing.T
	strm s4wave_world.SRPCEngineResourceService_WatchChangesClient
	msgs chan *s4wave_world.WatchChangesResponse
	errs chan error
}

// openChangeStream opens a raw WatchChanges stream on engine with the first
// request and reads its responses in the background.
func openChangeStream(
	ctx context.Context,
	t *testing.T,
	engine *sdk_world_engine.SDKEngine,
	first *s4wave_world.WatchChangesRequest,
) *changeStream {
	t.Helper()
	client, err := engine.GetResourceRef().GetClient()
	if err != nil {
		t.Fatal(err)
	}
	strm, err := s4wave_world.NewSRPCEngineResourceServiceClient(client).WatchChanges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = strm.Close() })
	if err := strm.Send(first); err != nil {
		t.Fatal(err)
	}
	s := &changeStream{t: t, strm: strm, msgs: make(chan *s4wave_world.WatchChangesResponse, 256), errs: make(chan error, 1)}
	go func() {
		for {
			msg, err := strm.Recv()
			if err != nil {
				s.errs <- err
				return
			}
			s.msgs <- msg
		}
	}()
	return s
}

// next returns the next response, failing the test after a timeout.
func (s *changeStream) next() *s4wave_world.WatchChangesResponse {
	s.t.Helper()
	select {
	case msg := <-s.msgs:
		return msg
	case err := <-s.errs:
		s.t.Fatalf("stream ended: %v", err)
	case <-time.After(5 * time.Second):
		s.t.Fatal("no response within 5s")
	}
	return nil
}

// nextChange returns the next change, failing on any other response.
func (s *changeStream) nextChange() *s4wave_world.WorldChange {
	s.t.Helper()
	change := s.next().GetChange()
	if change == nil {
		s.t.Fatal("got an ack, want a change")
	}
	return change
}

// commitObject creates key and returns the seqno of its commit.
func commitObject(ctx context.Context, t *testing.T, engine *sdk_world_engine.SDKEngine, key string) uint64 {
	t.Helper()
	ws := world.NewEngineWorldState(engine, true)
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// commitLink links from to to with pred and returns the seqno of its commit.
func commitLink(ctx context.Context, t *testing.T, engine *sdk_world_engine.SDKEngine, from, pred, to string) uint64 {
	t.Helper()
	ws := world.NewEngineWorldState(engine, true)
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(from, pred, to, "")); err != nil {
		t.Fatal(err)
	}
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// toggleObject deletes key, or creates it when absent, and returns the seqno
// of the commit.
func toggleObject(ctx context.Context, t *testing.T, engine *sdk_world_engine.SDKEngine, key string) uint64 {
	t.Helper()
	ws := world.NewEngineWorldState(engine, true)
	found, err := ws.DeleteObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		return commitObject(ctx, t, engine, key)
	}
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return seqno
}

// TestWatchChanges_HalfClose checks that a client sending one request without
// an interest and closing its send side receives every change, as a client
// built before interests does.
func TestWatchChanges_HalfClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{AfterSeqno: after})
	if err := s.strm.CloseSend(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"half-close/a", "half-close/b"} {
		seqno := commitObject(ctx, t, engine, key)
		change := s.nextChange()
		if change.GetSeqno() != seqno || !slices.Equal(change.GetKeys(), []string{key}) {
			t.Fatalf("change = %v, want %q at %d", change, key, seqno)
		}
	}
}

// TestWatchChanges_Named checks that a named interest receives only the
// changes naming its keys or matching its patterns, including a link to an
// object it did not name.
func TestWatchChanges_Named(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	for _, key := range []string{"named/origin", "named/member"} {
		commitObject(ctx, t, engine, key)
	}
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Follow one key and every link from the origin.
	link := world.NewGraphQuadWithKeys("named/origin", "<named-rel>", "", "")
	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{
		AfterSeqno: after,
		Interest:   s4wave_world.NewChangeInterestDelta(1, []string{"named/key"}, []world.GraphQuad{link}),
	})

	// An unnamed object is skipped; the named key arrives.
	commitObject(ctx, t, engine, "named/other")
	seqno := commitObject(ctx, t, engine, "named/key")
	if change := s.nextChange(); change.GetSeqno() != seqno {
		t.Fatalf("change = %v, want the named key at %d", change, seqno)
	}

	// A link from the origin arrives without its target being named.
	seqno = commitLink(ctx, t, engine, "named/origin", "<named-rel>", "named/member")
	if change := s.nextChange(); change.GetSeqno() != seqno || len(change.GetQuads()) == 0 {
		t.Fatalf("change = %v, want the link at %d", change, seqno)
	}
}

// TestWatchChanges_DeltaRace checks the swap at an ack: for a member added by
// a key, a member removed and a member matched only by a pattern, a commit
// landing between the delta and its ack is never lost, no change at or before
// the ack's seqno follows the ack, and a removed member's own commit is never
// sent after the ack, even coalesced with another key's. A member committed
// before the ack's seqno is TestWatchChanges_AckPastMember's case.
func TestWatchChanges_DeltaRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	commitObject(ctx, t, engine, "race/origin")
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{
		AfterSeqno: after,
		Interest:   &s4wave_world.ChangeInterestDelta{Version: 1, AddKeys: []string{"race/sentinel"}},
	})

	// state follows the stream: the last seqno seen, and the ack in force.
	var last, ackAt uint64
	version := uint64(1)
	// await reads until the ack for version, then until a change reaching
	// through, returning every change after the ack. A change at or before
	// the ack's seqno after it fails the test.
	await := func(through uint64) []*s4wave_world.WorldChange {
		t.Helper()
		var acked bool
		var changes []*s4wave_world.WorldChange
		for !acked || last < through {
			msg := s.next()
			if ack := msg.GetInterestAck(); ack != nil {
				if ack.GetVersion() != version || ack.GetAfterSeqno() < last || acked {
					t.Fatalf("ack = %v after seqno %d, want version %d", ack, last, version)
				}
				acked, ackAt, last = true, ack.GetAfterSeqno(), ack.GetAfterSeqno()
				continue
			}
			change := msg.GetChange()
			if change.GetSeqno() <= last {
				t.Fatalf("change at %d after %d", change.GetSeqno(), last)
			}
			last = change.GetSeqno()
			if acked {
				changes = append(changes, change)
			}
		}
		return changes
	}
	// sendDelta sends the next delta without waiting for its ack.
	sendDelta := func(delta *s4wave_world.ChangeInterestDelta) {
		t.Helper()
		version++
		delta.Version = version
		if err := s.strm.Send(&s4wave_world.WatchChangesRequest{Interest: delta}); err != nil {
			t.Fatal(err)
		}
	}
	// delivered reports whether changes carry a change through seqno naming
	// key or a quad from key.
	delivered := func(changes []*s4wave_world.WorldChange, seqno uint64, key string) bool {
		for _, change := range changes {
			if change.GetSeqno() < seqno {
				continue
			}
			if slices.Contains(change.GetKeys(), key) {
				return true
			}
			for _, q := range change.GetQuads() {
				if world.QuadToGraphQuad(q).GetSubject() == "<"+key+">" {
					return true
				}
			}
			return false
		}
		return false
	}

	for i := range 12 {
		member := "race/member-" + string(rune('a'+i))

		// Add a key and commit it at once: lost neither before nor after L.
		sendDelta(&s4wave_world.ChangeInterestDelta{AddKeys: []string{member}})
		added := commitObject(ctx, t, engine, member)
		sentinel := toggleObject(ctx, t, engine, "race/sentinel")
		changes := await(sentinel)
		if added > ackAt && !delivered(changes, added, member) {
			t.Fatalf("round %d: added member at %d lost after ack at %d", i, added, ackAt)
		}

		// Remove it and commit it at once: after L, its own commit is not sent.
		sendDelta(&s4wave_world.ChangeInterestDelta{RemoveKeys: []string{member}})
		ws := world.NewEngineWorldState(engine, true)
		if _, err := ws.DeleteObject(ctx, member); err != nil {
			t.Fatal(err)
		}
		sentinel = toggleObject(ctx, t, engine, "race/sentinel")
		changes = await(sentinel)
		for _, change := range changes {
			if slices.Contains(change.GetKeys(), member) {
				t.Fatalf("round %d: removed member sent at %d after ack at %d", i, change.GetSeqno(), ackAt)
			}
		}

		// Add a pattern and link at once: the link is not lost after L.
		rel := "<race-rel-" + string(rune('a'+i)) + ">"
		pattern := world.NewGraphQuadWithKeys("race/origin", rel, "", "")
		sendDelta(&s4wave_world.ChangeInterestDelta{AddPatterns: []*quad.Quad{{
			Subject:   pattern.GetSubject(),
			Predicate: pattern.GetPredicate(),
		}}})
		target := "race/target-" + string(rune('a'+i))
		commitObject(ctx, t, engine, target)
		linked := commitLink(ctx, t, engine, "race/origin", rel, target)
		sentinel = toggleObject(ctx, t, engine, "race/sentinel")
		changes = await(sentinel)
		if linked > ackAt && !delivered(changes, linked, "race/origin") {
			t.Fatalf("round %d: link at %d lost after ack at %d", i, linked, ackAt)
		}
	}
}

// TestWatchChanges_AckPastMember checks the case the ack rule exists for: a
// member's commit lands before a later change the earlier interest sends, so
// the ack's seqno L is past both the client's read revision R and the
// member's commit. The member's change is not sent, and the client must read
// the member again because L > R. It runs for a member added by key and for
// one matched only by a pattern.
func TestWatchChanges_AckPastMember(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	commitObject(ctx, t, engine, "ack/origin")
	read, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{
		AfterSeqno: read,
		Interest:   s4wave_world.NewChangeInterestDelta(1, []string{"ack/old"}, nil),
	})

	link := world.NewGraphQuadWithKeys("ack/origin", "<ack-rel>", "", "")
	cases := []struct {
		name   string
		delta  func(version uint64) *s4wave_world.ChangeInterestDelta
		commit func(n int) uint64
	}{{
		name: "key",
		delta: func(version uint64) *s4wave_world.ChangeInterestDelta {
			return s4wave_world.NewChangeInterestDelta(version, []string{"ack/member"}, nil)
		},
		commit: func(int) uint64 { return toggleObject(ctx, t, engine, "ack/member") },
	}, {
		name: "pattern",
		delta: func(version uint64) *s4wave_world.ChangeInterestDelta {
			return s4wave_world.NewChangeInterestDelta(version, nil, []world.GraphQuad{link})
		},
		commit: func(n int) uint64 {
			target := "ack/target-" + string(rune('a'+n))
			commitObject(ctx, t, engine, target)
			return commitLink(ctx, t, engine, "ack/origin", "<ack-rel>", target)
		},
	}}
	for i, c := range cases {
		version := uint64(i + 2)

		// The member commits, then the old key; the old key's change moves L
		// past the member before the delta is sent.
		member := c.commit(0)
		old := toggleObject(ctx, t, engine, "ack/old")
		if change := s.nextChange(); change.GetSeqno() != old {
			t.Fatalf("%s: change = %v, want the old key at %d", c.name, change, old)
		}
		if err := s.strm.Send(&s4wave_world.WatchChangesRequest{Interest: c.delta(version)}); err != nil {
			t.Fatal(err)
		}

		// The ack comes next: no change for the member precedes it, and its L
		// is past the read revision and the member's commit.
		ack := s.next().GetInterestAck()
		if ack.GetVersion() != version || ack.GetAfterSeqno() != old {
			t.Fatalf("%s: ack = %v, want version %d at %d", c.name, ack, version, old)
		}
		if ack.GetAfterSeqno() <= read || ack.GetAfterSeqno() <= member {
			t.Fatalf("%s: ack at %d, want past read %d and member %d", c.name, ack.GetAfterSeqno(), read, member)
		}
		read = ack.GetAfterSeqno()

		// The member's next commit arrives under the new interest; its earlier
		// commit was never sent.
		next := c.commit(1)
		if change := s.nextChange(); change.GetSeqno() != next {
			t.Fatalf("%s: change = %v, want the member at %d", c.name, change, next)
		}
	}
}

// TestWatchChanges_OldSeqno checks that a stream opened after a seqno the
// World no longer records starts with an unknown change.
func TestWatchChanges_OldSeqno(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Commit past the recorded heads.
	ws := world.NewEngineWorldState(engine, true)
	for i := range 140 {
		obj, err := ws.CreateObject(ctx, "old-seqno/"+string(rune('a'+i%26))+string(rune('a'+i/26)), nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{
		AfterSeqno: after,
		Interest:   s4wave_world.NewChangeInterestDelta(1, []string{"old-seqno/none"}, nil),
	})
	if change := s.nextChange(); !change.GetUnknown() {
		t.Fatalf("change = %v, want unknown", change)
	}
}

// TestWatchChanges_QuietDelta checks a delta after more commits than the World
// records, none of which the interest names: the ack's seqno reaches the head
// the watch last compared, not the last change sent, so the next watch starts
// from a recorded head and no unknown change follows.
func TestWatchChanges_QuietDelta(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	engine, cleanup := setupSDKEngine(ctx, t)
	defer cleanup()
	after, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{
		AfterSeqno: after,
		Interest:   s4wave_world.NewChangeInterestDelta(1, []string{"quiet/old"}, nil),
	})
	all := openChangeStream(ctx, t, engine, &s4wave_world.WatchChangesRequest{AfterSeqno: after})

	// Commit past the recorded heads without naming the interest, and wait
	// until the shared change watch compared the last commit.
	var prev, last uint64
	for i := range 130 {
		prev, last = last, commitObject(ctx, t, engine, "quiet/skip-"+strconv.Itoa(i))
	}
	for seqno := uint64(0); seqno < last; {
		seqno = all.nextChange().GetSeqno()
	}

	// The ack reaches the compared heads: the watch handed out at least the
	// commit before the last by the time the other stream saw the last.
	if err := s.strm.Send(&s4wave_world.WatchChangesRequest{
		Interest: s4wave_world.NewChangeInterestDelta(2, []string{"quiet/member"}, nil),
	}); err != nil {
		t.Fatal(err)
	}
	ack := s.next().GetInterestAck()
	if ack.GetVersion() != 2 || ack.GetAfterSeqno() < prev || ack.GetAfterSeqno() > last {
		t.Fatalf("ack = %v, want version 2 at %d or %d", ack, prev, last)
	}

	// The member's commit arrives next, known.
	member := commitObject(ctx, t, engine, "quiet/member")
	if change := s.nextChange(); change.GetUnknown() || change.GetSeqno() != member {
		t.Fatalf("change = %v, want the member at %d", change, member)
	}
}

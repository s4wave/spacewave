//go:build !js

package resource_world

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

type objectBodyPageWorld struct {
	world.WorldState
	bodies    []*world.ObjectBody
	readSizes []int
	pageSizes []int
	seqnos    []uint64
}

func (w *objectBodyPageWorld) GetObjectBodiesBatchPage(
	_ context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, error) {
	byKey := make(map[string]*world.ObjectBody, len(w.bodies))
	for _, body := range w.bodies {
		byKey[body.ObjectKey] = body
	}
	page := make([]*world.ObjectBody, 0, len(keys))
	readCount := 0
	for i, key := range keys {
		readCount++
		body := byKey[key]
		candidate := append(append([]*world.ObjectBody(nil), page...), body)
		if len(page) > 0 && encodedBodyResponseSize(candidate) > byteBudget {
			w.pageSizes = append(w.pageSizes, len(page))
			w.readSizes = append(w.readSizes, readCount)
			return page, uint32(i), nil
		}
		page = append(page, body)
	}
	w.pageSizes = append(w.pageSizes, len(page))
	w.readSizes = append(w.readSizes, readCount)
	return page, 0, nil
}

func (w *objectBodyPageWorld) GetObjectBodiesBatchPageWithSeqno(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	bodies, next, err := w.GetObjectBodiesBatchPage(ctx, keys, byteBudget)
	if err != nil {
		return nil, 0, 0, err
	}
	var seqno uint64
	if page := len(w.readSizes) - 1; page >= 0 && page < len(w.seqnos) {
		seqno = w.seqnos[page]
	}
	return bodies, next, seqno, nil
}

func encodedBodyResponseSize(bodies []*world.ObjectBody, nextKeyIndex ...uint32) int {
	out := make([]*s4wave_world.ObjectBody, len(bodies))
	for i, body := range bodies {
		out[i] = &s4wave_world.ObjectBody{
			ObjectKey: body.ObjectKey,
			Body:      body.Body,
			Exists:    body.Exists,
		}
	}
	var next uint32
	if len(nextKeyIndex) > 0 {
		next = nextKeyIndex[0]
	}
	return (&s4wave_world.GetObjectBodiesBatchResponse{
		Bodies:       out,
		NextKeyIndex: next,
	}).SizeVT()
}

func TestGetObjectBodiesBatchPageBudgetsEncodedResponseSize(t *testing.T) {
	ctx := context.Background()
	ws := &objectBodyPageWorld{}
	keys := make([]string, 32)
	for i := range keys {
		keys[i] = "body/tiny"
		ws.bodies = append(ws.bodies, &world.ObjectBody{
			ObjectKey: keys[i],
			Body:      []byte("x"),
			Exists:    true,
		})
	}

	page, next, _, err := getObjectBodiesBatchPage(ctx, ws, keys, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if next == 0 {
		t.Fatal("expected a continuation for many tiny bodies")
	}
	if len(page) < 2 {
		t.Fatalf("page length = %d, want multiple tiny bodies", len(page))
	}
	if size := encodedBodyResponseSize(page, next); size > 100 {
		t.Fatalf("encoded page size = %d, want <= 100", size)
	}
}

func TestGetObjectBodiesBatchPageReadsOnlyOneBoundedPageAtATime(t *testing.T) {
	ctx := context.Background()
	ws := &objectBodyPageWorld{}
	keys := make([]string, 32)
	for i := range keys {
		keys[i] = "body/" + string(rune('a'+i))
		ws.bodies = append(ws.bodies, &world.ObjectBody{
			ObjectKey: keys[i],
			Body:      []byte("x"),
			Exists:    true,
		})
	}

	var all []*world.ObjectBody
	for start := uint32(0); ; {
		page, next, _, err := getObjectBodiesBatchPage(ctx, ws, keys, start, 100)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if next == 0 {
			break
		}
		start = next
	}
	if len(all) != len(keys) {
		t.Fatalf("read %d bodies, want %d", len(all), len(keys))
	}
	if len(ws.readSizes) < 2 {
		t.Fatalf("read sizes = %v, want multiple owner reads", ws.readSizes)
	}
	remaining := len(keys)
	for i, read := range ws.readSizes {
		if i < len(ws.readSizes)-1 && read >= remaining {
			t.Fatalf("owner read %d read %d keys from suffix of %d", i, read, remaining)
		}
		remaining -= ws.pageSizes[i]
		if remaining < 0 {
			t.Fatalf("owner page sizes = %v exceed key count", ws.pageSizes)
		}
	}
}

func TestGetObjectBodiesBatchPageRejectsUint32IndexOverflow(t *testing.T) {
	ctx := context.Background()
	ws := &objectBodyPageWorld{
		bodies: []*world.ObjectBody{{ObjectKey: "body/only", Body: []byte("x"), Exists: true}},
	}
	resource := &WorldStateResource{ws: ws}

	resp, err := resource.GetObjectBodiesBatch(ctx, &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys:    []string{"body/only"},
		StartKeyIndex: ^uint32(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetBodies()) != 0 || resp.GetNextKeyIndex() != 0 {
		t.Fatalf("out-of-range response = %+v, want empty terminal page", resp)
	}
	if len(ws.readSizes) != 0 {
		t.Fatalf("out-of-range request performed reads: %v", ws.readSizes)
	}
}

func TestGetObjectBodiesBatchCarriesWorldSeqno(t *testing.T) {
	ws := &objectBodyPageWorld{
		bodies: []*world.ObjectBody{
			{ObjectKey: "body/one", Body: []byte("x"), Exists: true},
		},
		seqnos: []uint64{42},
	}
	resource := &WorldStateResource{ws: ws}

	resp, err := resource.GetObjectBodiesBatch(context.Background(), &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{"body/one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetWorldSeqno() != 42 {
		t.Fatalf("world seqno = %d, want 42", resp.GetWorldSeqno())
	}
}

func TestGetObjectBodiesBatchCarriesObjectRevisions(t *testing.T) {
	ws := &objectBodyPageWorld{
		bodies: []*world.ObjectBody{
			{ObjectKey: "body/one", Body: []byte("x"), Exists: true, Rev: 7},
			{ObjectKey: "body/two", Body: []byte("y"), Exists: true, Rev: 9},
		},
		seqnos: []uint64{42},
	}
	resource := &WorldStateResource{ws: ws}

	resp, err := resource.GetObjectBodiesBatch(context.Background(), &s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{"body/one", "body/two"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetBodies()) != 2 {
		t.Fatalf("body count = %d, want 2", len(resp.GetBodies()))
	}
	for i, want := range []uint64{7, 9} {
		if got := resp.GetBodies()[i].GetRev(); got != want {
			t.Fatalf("body %d rev = %d, want %d", i, got, want)
		}
	}
}

var _ world.ObjectBodyPageBatcher = (*objectBodyPageWorld)(nil)
var _ world.ObjectBodyPageSeqnoBatcher = (*objectBodyPageWorld)(nil)

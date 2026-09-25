//go:build !js

package resource_world

import (
	"context"
	"slices"
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

func encodedBodyResponseSize(bodies []*world.ObjectBody) int {
	out := make([]*s4wave_world.ObjectBody, len(bodies))
	for i, body := range bodies {
		out[i] = &s4wave_world.ObjectBody{
			ObjectKey: body.ObjectKey,
			Body:      body.Body,
			Exists:    body.Exists,
		}
	}
	return (&s4wave_world.GetObjectBodiesBatchResponse{Bodies: out}).SizeVT()
}

// objectBodiesPageStream collects the pages a GetObjectBodiesBatch handler
// sends.
type objectBodiesPageStream struct {
	s4wave_world.SRPCWorldStateResourceService_GetObjectBodiesBatchStream
	ctx   context.Context
	pages []*s4wave_world.GetObjectBodiesBatchResponse
}

func (s *objectBodiesPageStream) Context() context.Context {
	return s.ctx
}

func (s *objectBodiesPageStream) Send(resp *s4wave_world.GetObjectBodiesBatchResponse) error {
	s.pages = append(s.pages, resp)
	return nil
}

func TestStreamObjectBodyPagesSendsBoundedPagesInOrder(t *testing.T) {
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

	var pages []*s4wave_world.GetObjectBodiesBatchResponse
	err := streamObjectBodyPages(context.Background(), ws, keys, 100, func(resp *s4wave_world.GetObjectBodiesBatchResponse) error {
		pages = append(pages, resp)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 2 {
		t.Fatalf("page count = %d, want multiple pages for many tiny bodies", len(pages))
	}

	var got []string
	for i, page := range pages {
		if size := page.SizeVT(); size > 100 {
			t.Fatalf("page %d encoded size = %d, want <= 100", i, size)
		}
		for _, body := range page.GetBodies() {
			got = append(got, body.GetObjectKey())
		}
	}
	if !slices.Equal(got, keys) {
		t.Fatalf("streamed keys = %v, want %v", got, keys)
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

func TestGetObjectBodiesBatchCarriesWorldSeqno(t *testing.T) {
	ws := &objectBodyPageWorld{
		bodies: []*world.ObjectBody{
			{ObjectKey: "body/one", Body: []byte("x"), Exists: true},
		},
		seqnos: []uint64{42},
	}
	resource := &WorldStateResource{ws: ws}
	strm := &objectBodiesPageStream{ctx: context.Background()}

	err := resource.GetObjectBodiesBatch(&s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{"body/one"},
	}, strm)
	if err != nil {
		t.Fatal(err)
	}
	if len(strm.pages) != 1 {
		t.Fatalf("page count = %d, want 1", len(strm.pages))
	}
	if got := strm.pages[0].GetWorldSeqno(); got != 42 {
		t.Fatalf("world seqno = %d, want 42", got)
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
	strm := &objectBodiesPageStream{ctx: context.Background()}

	err := resource.GetObjectBodiesBatch(&s4wave_world.GetObjectBodiesBatchRequest{
		ObjectKeys: []string{"body/one", "body/two"},
	}, strm)
	if err != nil {
		t.Fatal(err)
	}
	if len(strm.pages) != 1 || len(strm.pages[0].GetBodies()) != 2 {
		t.Fatalf("pages = %+v, want one page with 2 bodies", strm.pages)
	}
	for i, want := range []uint64{7, 9} {
		if got := strm.pages[0].GetBodies()[i].GetRev(); got != want {
			t.Fatalf("body %d rev = %d, want %d", i, got, want)
		}
	}
}

var (
	_ world.ObjectBodyPageBatcher      = (*objectBodyPageWorld)(nil)
	_ world.ObjectBodyPageSeqnoBatcher = (*objectBodyPageWorld)(nil)
)

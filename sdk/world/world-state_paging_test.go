package s4wave_world

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// objectBodiesBatchService answers each GetObjectBodiesBatch call with the
// next scripted stream of pages.
type objectBodiesBatchService struct {
	SRPCWorldStateResourceServiceClient
	streams  [][]*GetObjectBodiesBatchResponse
	requests []*GetObjectBodiesBatchRequest
}

func (s *objectBodiesBatchService) GetObjectBodiesBatch(
	_ context.Context,
	req *GetObjectBodiesBatchRequest,
) (SRPCWorldStateResourceService_GetObjectBodiesBatchClient, error) {
	s.requests = append(s.requests, req)
	return &objectBodiesBatchClient{pages: s.streams[len(s.requests)-1]}, nil
}

// objectBodiesBatchClient replays pages, then ends the stream.
type objectBodiesBatchClient struct {
	SRPCWorldStateResourceService_GetObjectBodiesBatchClient
	pages []*GetObjectBodiesBatchResponse
}

func (c *objectBodiesBatchClient) Recv() (*GetObjectBodiesBatchResponse, error) {
	if len(c.pages) == 0 {
		return nil, io.EOF
	}
	page := c.pages[0]
	c.pages = c.pages[1:]
	return page, nil
}

func (c *objectBodiesBatchClient) Close() error {
	return nil
}

func TestWorldStateForEachObjectBodyPageYieldsPages(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}, {ObjectKey: "body/two"}}},
			{Bodies: []*ObjectBody{{ObjectKey: "body/three"}}},
		}},
	}
	ws := &WorldState{service: service}

	var pages [][]string
	err := ws.ForEachObjectBodyPage(context.Background(), []string{"body/one", "body/two", "body/three"}, func(bodies []*world.ObjectBody) error {
		page := make([]string, len(bodies))
		for i, body := range bodies {
			page[i] = body.ObjectKey
		}
		pages = append(pages, page)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(pages))
	}
	if got, want := pages[0], []string{"body/one", "body/two"}; !slices.Equal(got, want) {
		t.Fatalf("first page = %v, want %v", got, want)
	}
	if got, want := pages[1], []string{"body/three"}; !slices.Equal(got, want) {
		t.Fatalf("second page = %v, want %v", got, want)
	}
}

func TestWorldStateObjectBodyPagePreservesRevisions(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{{
			{
				Bodies: []*ObjectBody{
					{ObjectKey: "body/one", Exists: true, Rev: 7},
					{ObjectKey: "body/two", Exists: true, Rev: 9},
				},
			},
		}},
	}
	ws := &WorldState{service: service}

	var revs []uint64
	err := ws.ForEachObjectBodyPage(context.Background(), []string{"body/one", "body/two"}, func(bodies []*world.ObjectBody) error {
		for _, body := range bodies {
			revs = append(revs, body.Rev)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := revs, []uint64{7, 9}; !slices.Equal(got, want) {
		t.Fatalf("revs = %v, want %v", got, want)
	}
}

func TestWorldStateGetObjectBodiesBatchChunksRequestKeys(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{
			{{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}}},
			{{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}}},
		},
	}
	ws := &WorldState{service: service}
	keySize := (block.MaxBlockSize - 64*1024) / 2
	keys := []string{strings.Repeat("a", keySize), strings.Repeat("b", keySize)}

	bodies, err := ws.GetObjectBodiesBatch(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(service.requests))
	}
	for i, req := range service.requests {
		if got := req.SizeVT(); got > block.MaxBlockSize-64*1024 {
			t.Fatalf("request %d encoded size = %d, exceeds budget %d", i, got, block.MaxBlockSize-64*1024)
		}
	}
	if len(service.requests[0].GetObjectKeys()) != 1 || len(service.requests[1].GetObjectKeys()) != 1 {
		t.Fatalf("request key counts = %d, %d, want 1, 1", len(service.requests[0].GetObjectKeys()), len(service.requests[1].GetObjectKeys()))
	}
	if len(bodies) != 2 || bodies[0].ObjectKey != "body/one" || bodies[1].ObjectKey != "body/two" {
		t.Fatalf("bodies = %+v, want both chunk results in order", bodies)
	}
}

func TestChunkObjectBodyKeysIncrementalSizeMatchesRequest(t *testing.T) {
	budget := world.ObjectBodiesBatchByteBudget
	cases := [][]string{
		{
			"",
			"a",
			strings.Repeat("b", 126),
			strings.Repeat("c", 127),
			strings.Repeat("d", 128),
			strings.Repeat("e", 16382),
			strings.Repeat("f", 16383),
			strings.Repeat("g", 16384),
		},
		{
			strings.Repeat("h", budget/2-8),
			strings.Repeat("i", budget/2-8),
		},
		{
			strings.Repeat("j", budget-16),
			"tail",
		},
	}

	for caseIndex, keys := range cases {
		chunks, err := chunkObjectBodyKeys(keys)
		if err != nil {
			t.Fatalf("case %d: chunk keys: %v", caseIndex, err)
		}
		var flattened []string
		for chunkIndex, chunk := range chunks {
			incrementalSize := 0
			for _, key := range chunk {
				incrementalSize += protobuf_go_lite.SizeStringValue(1, key)
			}
			request := &GetObjectBodiesBatchRequest{ObjectKeys: chunk}
			if got := request.SizeVT(); got != incrementalSize {
				t.Fatalf(
					"case %d chunk %d encoded size = %d, incremental size = %d",
					caseIndex,
					chunkIndex,
					got,
					incrementalSize,
				)
			}
			if incrementalSize > budget {
				t.Fatalf("case %d chunk %d size = %d, exceeds budget %d", caseIndex, chunkIndex, incrementalSize, budget)
			}
			flattened = append(flattened, chunk...)
		}
		if !slices.Equal(flattened, keys) {
			t.Fatalf("case %d flattened keys = %v, want %v", caseIndex, flattened, keys)
		}
	}
}

func TestChunkObjectBodyKeysRejectsOversizedSingleKey(t *testing.T) {
	key := strings.Repeat("z", world.ObjectBodiesBatchByteBudget)
	_, err := chunkObjectBodyKeys([]string{key})
	if err == nil {
		t.Fatal("expected a single oversized key to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds request byte budget") {
		t.Fatalf("oversized key error = %v, want request budget error", err)
	}
}

func TestWorldStateGetObjectBodiesBatchPagesResults(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{{
			{Bodies: []*ObjectBody{{ObjectKey: "body/large", Body: []byte("12345"), Exists: true}}},
			{
				Bodies: []*ObjectBody{
					{ObjectKey: "body/missing", Exists: false},
					{ObjectKey: "body/large", Body: []byte("12345"), Exists: true},
				},
			},
		}},
	}
	ws := &WorldState{service: service}
	keys := []string{"body/large", "body/missing", "body/large"}

	bodies, err := ws.GetObjectBodiesBatch(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(service.requests))
	}
	if len(bodies) != len(keys) {
		t.Fatalf("body count = %d, want %d", len(bodies), len(keys))
	}
	for i, want := range keys {
		if bodies[i].ObjectKey != want {
			t.Fatalf("body %d key = %q, want %q", i, bodies[i].ObjectKey, want)
		}
	}
	if bodies[1].Exists || bodies[1].Body != nil {
		t.Fatalf("missing body = %+v, want an empty missing marker", bodies[1])
	}
}

func TestWorldStateGetObjectBodiesBatchRestartsOnWorldSeqnoChange(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{
			{
				{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, WorldSeqno: 1},
				{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
			},
			{
				{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, WorldSeqno: 2},
				{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
			},
		},
	}
	ws := &WorldState{service: service}

	bodies, err := ws.GetObjectBodiesBatch(context.Background(), []string{"body/one", "body/two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(service.requests))
	}
	if len(bodies) != 2 || bodies[0].ObjectKey != "body/one" || bodies[1].ObjectKey != "body/two" {
		t.Fatalf("bodies = %+v, want both consistent pages", bodies)
	}
}

func TestWorldStateGetObjectBodiesBatchReturnsTypedRevisionError(t *testing.T) {
	service := &objectBodiesBatchService{}
	for attempt := range uint64(4) {
		service.streams = append(service.streams, []*GetObjectBodiesBatchResponse{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, WorldSeqno: 2*attempt + 1},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2*attempt + 2},
		})
	}
	ws := &WorldState{service: service}

	_, err := ws.GetObjectBodiesBatch(context.Background(), []string{"body/one", "body/two"})
	var revisionErr *ObjectBodiesBatchRevisionError
	if !errors.As(err, &revisionErr) {
		t.Fatalf("error = %v, want ObjectBodiesBatchRevisionError", err)
	}
	if revisionErr.Retries != 3 {
		t.Fatalf("retries = %d, want 3", revisionErr.Retries)
	}
}

func TestWorldStateForEachObjectBodyPageReturnsRevisionError(t *testing.T) {
	service := &objectBodiesBatchService{
		streams: [][]*GetObjectBodiesBatchResponse{{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, WorldSeqno: 1},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
		}},
	}
	ws := &WorldState{service: service}

	pages := 0
	err := ws.ForEachObjectBodyPage(context.Background(), []string{"body/one", "body/two"}, func([]*world.ObjectBody) error {
		pages++
		return nil
	})
	var revisionErr *ObjectBodiesBatchRevisionError
	if !errors.As(err, &revisionErr) {
		t.Fatalf("error = %v, want ObjectBodiesBatchRevisionError", err)
	}
	if revisionErr.Expected != 1 || revisionErr.Got != 2 {
		t.Fatalf("revision error = %+v, want expected 1 and got 2", revisionErr)
	}
	if pages != 1 {
		t.Fatalf("page callbacks = %d, want 1 before the revision error", pages)
	}
}

package s4wave_world

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

type objectBodiesBatchService struct {
	SRPCWorldStateResourceServiceClient
	responses []*GetObjectBodiesBatchResponse
	requests  []*GetObjectBodiesBatchRequest
}

func (s *objectBodiesBatchService) GetObjectBodiesBatch(_ context.Context, req *GetObjectBodiesBatchRequest) (*GetObjectBodiesBatchResponse, error) {
	s.requests = append(s.requests, req)
	return s.responses[len(s.requests)-1], nil
}

func TestWorldStateForEachObjectBodyPageYieldsPages(t *testing.T) {
	service := &objectBodiesBatchService{
		responses: []*GetObjectBodiesBatchResponse{
			{
				Bodies:       []*ObjectBody{{ObjectKey: "body/one"}, {ObjectKey: "body/two"}},
				NextKeyIndex: 2,
			},
			{
				Bodies: []*ObjectBody{{ObjectKey: "body/three"}},
			},
		},
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

func TestWorldStateGetObjectBodiesBatchChunksRequestKeys(t *testing.T) {
	service := &objectBodiesBatchService{
		responses: []*GetObjectBodiesBatchResponse{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}},
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
	const maxStartKeyIndex = ^uint32(0)
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
			incrementalSize := protobuf_go_lite.SizeVarintValue(1, maxStartKeyIndex)
			for _, key := range chunk {
				incrementalSize += protobuf_go_lite.SizeStringValue(1, key)
			}
			request := &GetObjectBodiesBatchRequest{
				ObjectKeys:    chunk,
				StartKeyIndex: maxStartKeyIndex,
			}
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
		responses: []*GetObjectBodiesBatchResponse{
			{
				Bodies:       []*ObjectBody{{ObjectKey: "body/large", Body: []byte("12345"), Exists: true}},
				NextKeyIndex: 1,
			},
			{
				Bodies: []*ObjectBody{
					{ObjectKey: "body/missing", Exists: false},
					{ObjectKey: "body/large", Body: []byte("12345"), Exists: true},
				},
			},
		},
	}
	ws := &WorldState{service: service}
	keys := []string{"body/large", "body/missing", "body/large"}

	bodies, err := ws.GetObjectBodiesBatch(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(service.requests))
	}
	if service.requests[0].GetStartKeyIndex() != 0 || service.requests[1].GetStartKeyIndex() != 1 {
		t.Fatalf("request start indexes = %d, %d, want 0, 1", service.requests[0].GetStartKeyIndex(), service.requests[1].GetStartKeyIndex())
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
		responses: []*GetObjectBodiesBatchResponse{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 1},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 2},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
		},
	}
	ws := &WorldState{service: service}

	bodies, err := ws.GetObjectBodiesBatch(context.Background(), []string{"body/one", "body/two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 4 {
		t.Fatalf("request count = %d, want 4", len(service.requests))
	}
	for i, want := range []uint32{0, 1, 0, 1} {
		if got := service.requests[i].GetStartKeyIndex(); got != want {
			t.Fatalf("request %d start index = %d, want %d", i, got, want)
		}
	}
	if len(bodies) != 2 || bodies[0].ObjectKey != "body/one" || bodies[1].ObjectKey != "body/two" {
		t.Fatalf("bodies = %+v, want both consistent pages", bodies)
	}
}

func TestWorldStateGetObjectBodiesBatchReturnsTypedRevisionError(t *testing.T) {
	service := &objectBodiesBatchService{
		responses: []*GetObjectBodiesBatchResponse{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 1},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, NextKeyIndex: 1, WorldSeqno: 2},
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 3},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, NextKeyIndex: 1, WorldSeqno: 4},
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 5},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, NextKeyIndex: 1, WorldSeqno: 6},
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 7},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 8},
		},
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
		responses: []*GetObjectBodiesBatchResponse{
			{Bodies: []*ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 1},
			{Bodies: []*ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
		},
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

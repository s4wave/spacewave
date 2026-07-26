package sdk_world_engine

import (
	"context"
	"testing"

	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

type objectBodiesBatchService struct {
	s4wave_world.SRPCWorldStateResourceServiceClient
	responses []*s4wave_world.GetObjectBodiesBatchResponse
	requests  []*s4wave_world.GetObjectBodiesBatchRequest
}

func (s *objectBodiesBatchService) GetObjectBodiesBatch(_ context.Context, req *s4wave_world.GetObjectBodiesBatchRequest) (*s4wave_world.GetObjectBodiesBatchResponse, error) {
	s.requests = append(s.requests, req)
	return s.responses[len(s.requests)-1], nil
}

func TestSDKWorldStateGetObjectBodiesBatchPagesResults(t *testing.T) {
	service := &objectBodiesBatchService{
		responses: []*s4wave_world.GetObjectBodiesBatchResponse{
			{
				Bodies:       []*s4wave_world.ObjectBody{{ObjectKey: "body/large", Body: []byte("12345"), Exists: true}},
				NextKeyIndex: 1,
			},
			{
				Bodies: []*s4wave_world.ObjectBody{
					{ObjectKey: "body/missing", Exists: false},
					{ObjectKey: "body/large", Body: []byte("12345"), Exists: true},
				},
			},
		},
	}
	ws := &SDKWorldState{service: service}
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

func TestSDKWorldStateGetObjectBodiesBatchRestartsOnWorldSeqnoChange(t *testing.T) {
	service := &objectBodiesBatchService{
		responses: []*s4wave_world.GetObjectBodiesBatchResponse{
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 1},
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/one"}}, NextKeyIndex: 1, WorldSeqno: 2},
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/two"}}, WorldSeqno: 2},
		},
	}
	ws := &SDKWorldState{service: service}

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

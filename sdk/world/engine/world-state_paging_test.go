package sdk_world_engine

import (
	"context"
	"io"
	"testing"

	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// objectBodiesBatchService answers GetObjectBodiesBatch with a scripted stream
// of pages.
type objectBodiesBatchService struct {
	s4wave_world.SRPCWorldStateResourceServiceClient
	pages    []*s4wave_world.GetObjectBodiesBatchResponse
	requests []*s4wave_world.GetObjectBodiesBatchRequest
}

func (s *objectBodiesBatchService) GetObjectBodiesBatch(
	_ context.Context,
	req *s4wave_world.GetObjectBodiesBatchRequest,
) (s4wave_world.SRPCWorldStateResourceService_GetObjectBodiesBatchClient, error) {
	s.requests = append(s.requests, req)
	return &objectBodiesBatchClient{pages: s.pages}, nil
}

// objectBodiesBatchClient replays pages, then ends the stream.
type objectBodiesBatchClient struct {
	s4wave_world.SRPCWorldStateResourceService_GetObjectBodiesBatchClient
	pages []*s4wave_world.GetObjectBodiesBatchResponse
}

func (c *objectBodiesBatchClient) Recv() (*s4wave_world.GetObjectBodiesBatchResponse, error) {
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

func TestSDKWorldStateGetObjectBodiesBatchCollectsStreamedPages(t *testing.T) {
	service := &objectBodiesBatchService{
		pages: []*s4wave_world.GetObjectBodiesBatchResponse{
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/large", Body: []byte("12345"), Exists: true}}},
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

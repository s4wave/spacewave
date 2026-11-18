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

func TestSDKWorldStateGetObjectBodiesBatchPageReadsOnePage(t *testing.T) {
	// Script two pages; a one-page read must stop after the first.
	service := &objectBodiesBatchService{
		pages: []*s4wave_world.GetObjectBodiesBatchResponse{
			{
				Bodies: []*s4wave_world.ObjectBody{
					{ObjectKey: "body/large", Exists: true, WithheldSize: 9 << 20},
					{ObjectKey: "body/missing"},
				},
				WorldSeqno: 7,
			},
			{Bodies: []*s4wave_world.ObjectBody{{ObjectKey: "body/small", Body: []byte("1"), Exists: true}}},
		},
	}
	ws := &SDKWorldState{service: service}
	keys := []string{"body/large", "body/missing", "body/small"}

	// Read one page through the World state page method.
	bodies, consumed, seqno, err := ws.GetObjectBodiesBatchPage(context.Background(), keys, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	// One request carries the page budget; the page answers two keys and
	// keeps the withheld entry's size.
	if len(service.requests) != 1 || service.requests[0].GetPageBudget() != 1<<20 {
		t.Fatalf("requests = %+v, want one with page budget %d", service.requests, 1<<20)
	}
	if len(bodies) != 2 || consumed != 2 || seqno != 7 {
		t.Fatalf("page = %d bodies, consumed %d, seqno %d; want 2, 2, 7", len(bodies), consumed, seqno)
	}
	if bodies[0].WithheldSize != 9<<20 || bodies[0].Body != nil || bodies[1].Exists {
		t.Fatalf("bodies = %+v, want a withheld entry and a missing marker", bodies)
	}
}

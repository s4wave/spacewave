package dex_solicit

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

// TestStoreReadWaitsForSolicitation proves a read issued as soon as a link
// comes up reaches the linked peer instead of missing before its DEX session
// exists.
func TestStoreReadWaitsForSolicitation(t *testing.T) {
	// Bound the test and share one protocol context between the nodes.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	protocolContext := []byte("shared-object")

	// Start the reader and writer nodes.
	reader := newTestRelayNode(t, ctx, le, "reader", protocolContext, 0)
	writer := newTestRelayNode(t, ctx, le, "writer", protocolContext, 0)

	// Store the block only in the writer's bucket.
	writerBucket, _, writerBucketRef, err := bucket.ExBuildBucketAPI(
		ctx,
		writer.tb.Bus,
		false,
		writer.bucketID,
		writer.tb.Volume.GetID(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writerBucketRef.Release)
	data := []byte("writer block")
	ref, _, err := writerBucket.GetBucket().PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Ask for the link before it exists. The solicitation controller holds a
	// weak reference to the request, so it observes the link before the test
	// does, as it would observe a link before any stream crossed it.
	writerTransport, err := writer.transport.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writerPeerID := writerTransport.(*inproc.Inproc).GetPeerID()
	linked := make(chan struct{}, 1)
	_, linkRef, err := reader.tb.Bus.AddDirective(
		link.NewEstablishLinkWithPeer("", writerPeerID),
		directive.NewTypedCallbackHandler(
			func(directive.TypedAttachedValue[link.MountedLink]) {
				select {
				case linked <- struct{}{}:
				default:
				}
			},
			nil, nil, nil,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(linkRef.Release)

	// Bring up the link, then read before any DEX session can exist.
	connectTestRelayNodes(t, ctx, reader, writer)
	select {
	case <-linked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Read the block and expect the writer's copy.
	got, found, err := NewStore(reader.dex).GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("read missed the block held by the linked writer")
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("read data = %q, want %q", got, data)
	}
}

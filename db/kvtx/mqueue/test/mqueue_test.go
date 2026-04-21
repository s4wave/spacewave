package kvtx_mqueuetest

import (
	"context"
	"fmt"
	"testing"

	kvtx_mqueue "github.com/s4wave/spacewave/db/kvtx/mqueue"
	object_mock "github.com/s4wave/spacewave/db/object/mock"
)

// TestMQueueSimple is a simple mqueue test.
func TestMQueueSimple(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	ctx := context.Background()
	q := kvtx_mqueue.NewMQueue(objs, &kvtx_mqueue.Config{})
	for i := 1; i <= 3; i++ {
		msg, err := q.Push(ctx, fmt.Appendf(nil, "test-%d", i))
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("enqueued message %v", msg.GetId())
	}
	nmsg := 0
	for {
		msg, ok, err := q.Peek(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !ok {
			break
		}
		nmsg++
		t.Logf("peek/ack message %d: %s", msg.GetId(), string(msg.GetData()))
		if err := q.Ack(ctx, msg.GetId()); err != nil {
			t.Fatal(err.Error())
		}
	}
	if nmsg != 3 {
		t.Fail()
	}
}

package object_mqueue

import (
	"context"
	"fmt"
	"github.com/aperturerobotics/hydra/object/mock"
	"testing"
)

// TestMQueueSimple is a simple mqueue test.
func TestMQueueSimple(t *testing.T) {
	objs, _ := object_mock.BuildTestStore(t)
	q := NewMQueue(context.Background(), objs)
	for i := 1; i <= 3; i++ {
		msg, err := q.Push([]byte(fmt.Sprintf("test-%d", i)))
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("enqueued message %v", msg.GetId())
	}
	nmsg := 0
	for {
		msg, ok, err := q.Peek()
		if err != nil {
			t.Fatal(err.Error())
		}
		if !ok {
			break
		}
		nmsg++
		t.Logf("peek/ack message %d: %s", msg.GetId(), string(msg.GetData()))
		if err := q.Ack(msg.GetId()); err != nil {
			t.Fatal(err.Error())
		}
	}
	if nmsg != 3 {
		t.Fail()
	}
}

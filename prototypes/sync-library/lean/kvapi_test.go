//go:build !js

package lean

import (
	"context"
	"testing"
	"time"
)

func TestKvApi(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	if err := KvOpen(ctx); err != nil {
		t.Fatal(err.Error())
	}
	defer func() {
		KvClose()
	}()

	if err := KvPut("user/1", `{"name":"ada"}`); err != nil {
		t.Fatal(err.Error())
	}
	got, found, err := KvGet("user/1")
	if err != nil || !found || got != `{"name":"ada"}` {
		t.Fatalf("KvGet = %q %v %v", got, found, err)
	}

	snapshots := make(chan string, 8)
	if err := KvWatch("user/", func(snapshot string) {
		select {
		case snapshots <- snapshot:
		default:
		}
	}); err != nil {
		t.Fatal(err.Error())
	}
	if err := KvPut("user/2", `{"name":"bob"}`); err != nil {
		t.Fatal(err.Error())
	}
	select {
	case s := <-snapshots:
		if s == "" {
			t.Fatal("empty snapshot")
		}
	case <-time.After(10000000000):
		t.Fatal("timed out waiting for watch snapshot")
	}

	listed, err := KvList("user/")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(listed) < 40 {
		t.Fatalf("KvList too short: %s", listed)
	}

	KvStopWatches()
	if err := KvDelete("user/1"); err != nil {
		t.Fatal(err.Error())
	}
	if _, found, _ := KvGet("user/1"); found {
		t.Fatal("expected deletion")
	}
	KvClose()
	KvClose() // idempotent
}

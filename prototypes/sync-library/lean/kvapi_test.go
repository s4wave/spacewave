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
	got, err := KvGet("user/1")
	if err != nil || got != `{"name":"ada"}` {
		t.Fatalf("KvGet = %q %v", got, err)
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
	if got, _ := KvGet("user/1"); got != "" {
		t.Fatalf("expected deletion, got %q", got)
	}
	KvClose()
	KvClose() // idempotent
}

func TestKvDurableReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60000000000)
	defer cancel()
	dir := t.TempDir()

	if err := KvOpenDurable(ctx, dir); err != nil {
		t.Fatal(err.Error())
	}
	if err := KvPut("durable/a", `one`); err != nil {
		t.Fatal(err.Error())
	}
	if err := KvPut("durable/b", `two`); err != nil {
		t.Fatal(err.Error())
	}
	KvClose()

	if got, _ := KvGet("durable/a"); got != "" {
		t.Fatalf("expected empty store after close, got %q", got)
	}

	if err := KvOpenDurable(ctx, dir); err != nil {
		t.Fatal(err.Error())
	}
	got, err := KvGet("durable/a")
	if err != nil || got != "one" {
		t.Fatalf("KvGet after reopen = %q %v; want one", got, err)
	}
	got2, err := KvGet("durable/b")
	if err != nil || got2 != "two" {
		t.Fatalf("KvGet b after reopen = %q %v", got2, err)
	}
	KvClose()
	KvOpen(context.Background())
	KvClose()
}

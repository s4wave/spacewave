package resource_bucket_lookup

import (
	"context"
	"testing"

	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/testbed"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	"github.com/sirupsen/logrus"
)

func TestUnmarshalUsesCursorRefWhenRequestRefEmpty(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(cursor.Release)

	want := &block_mock.Example{Msg: "manifest data"}
	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(want, true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	cursor.SetRootRef(rootRef)

	resource := NewBucketLookupCursorResource(le, tb.Bus, cursor)
	got, err := resource.Unmarshal(ctx, &s4wave_bucket_lookup.UnmarshalRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if !got.GetFound() {
		t.Fatal("expected block data to be found")
	}

	example := &block_mock.Example{}
	if err := example.UnmarshalBlock(got.GetData()); err != nil {
		t.Fatal(err.Error())
	}
	if example.GetMsg() != want.GetMsg() {
		t.Fatalf("message = %q, want %q", example.GetMsg(), want.GetMsg())
	}
}

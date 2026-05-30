package s4wave_terminal

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
)

func TestTerminalValidatePinsDeviceTarget(t *testing.T) {
	term := &Terminal{
		Name:            "Build Host Shell",
		DeviceObjectKey: "devices/build-host",
		DevicePeerId:    "12D3KooWDevice",
		Environment:     []string{"TERM=xterm-256color"},
	}
	if err := term.Validate(); err != nil {
		t.Fatalf("valid terminal failed validation: %v", err)
	}

	term.DevicePeerId = ""
	if err := term.Validate(); err == nil {
		t.Fatal("expected missing device peer id to fail validation")
	}

	term.DevicePeerId = "12D3KooWDevice"
	term.Environment = []string{"BROKEN"}
	if err := term.Validate(); err == nil {
		t.Fatal("expected malformed environment entry to fail validation")
	}

	term.Environment = []string{"=value"}
	if err := term.Validate(); err == nil {
		t.Fatal("expected empty environment key to fail validation")
	}
}

func TestCreateTerminalOpValidate(t *testing.T) {
	op := NewCreateTerminalOp(
		"terminal/build-host-1",
		"Build Host Shell",
		"devices/build-host",
		"12D3KooWDevice",
		time.Unix(10, 0),
	)
	if err := op.Validate(); err != nil {
		t.Fatalf("valid create terminal op failed validation: %v", err)
	}

	op.ObjectKey = ""
	if err := op.Validate(); err != world.ErrEmptyObjectKey {
		t.Fatalf("missing object key error = %v, want %v", err, world.ErrEmptyObjectKey)
	}
}

func TestNormalizeTerminalFrameSize(t *testing.T) {
	cols, rows := NormalizeTerminalFrameSize(0, 0)
	if cols != DefaultTerminalCols || rows != DefaultTerminalRows {
		t.Fatalf("default size = %dx%d", cols, rows)
	}

	cols, rows = NormalizeTerminalFrameSize(120, 40)
	if cols != 120 || rows != 40 {
		t.Fatalf("explicit size = %dx%d", cols, rows)
	}
}

func TestTerminalMarshalBlockRoundTrip(t *testing.T) {
	term := &Terminal{
		Name:            "Build Host Shell",
		DeviceObjectKey: "devices/build-host",
		DevicePeerId:    "12D3KooWDevice",
		Cols:            120,
		Rows:            40,
		CreatedAt:       timestamppb.New(time.Unix(10, 0)),
	}
	data, err := term.MarshalBlock()
	if err != nil {
		t.Fatalf("MarshalBlock() error = %v", err)
	}

	got := &Terminal{}
	if err := got.UnmarshalBlock(data); err != nil {
		t.Fatalf("UnmarshalBlock() error = %v", err)
	}
	if !got.EqualVT(term) {
		t.Fatalf("round trip = %#v, want %#v", got, term)
	}
}

func TestLookupCreateTerminalOp(t *testing.T) {
	op, err := LookupCreateTerminalOp(context.Background(), CreateTerminalOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*CreateTerminalOp); !ok {
		t.Fatalf("expected CreateTerminalOp, got %T", op)
	}
}

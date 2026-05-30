package objecttypes

import (
	"context"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

func TestLookupDeviceObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_device.DeviceTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Device ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_device.DeviceTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_device.DeviceTypeID)
	}
}

func TestLookupComputersDashboardObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_device.ComputersDashboardTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Computers dashboard ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_device.ComputersDashboardTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_device.ComputersDashboardTypeID)
	}
}

func TestLookupTerminalObjectType(t *testing.T) {
	got, err := LookupObjectType(context.Background(), s4wave_terminal.TerminalTypeID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected Terminal ObjectType")
	}
	if got.GetObjectTypeID() != s4wave_terminal.TerminalTypeID {
		t.Fatalf("object type id = %q, want %q", got.GetObjectTypeID(), s4wave_terminal.TerminalTypeID)
	}
}

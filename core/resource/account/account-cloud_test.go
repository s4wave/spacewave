package resource_account

import (
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// TestBuildCloudSessionRows verifies cloud session rows normalize timestamps,
// current-session state, and mirrored metadata onto the shared DTO.
func TestBuildCloudSessionRows(t *testing.T) {
	rows := []*api.AccountSessionInfo{
		{
			PeerId:     "peer-current",
			CreatedAt:  time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC).UnixMilli(),
			LastSeen:   time.Date(2026, 4, 16, 8, 30, 0, 0, time.UTC).UnixMilli(),
			DeviceInfo: "macOS",
		},
		{
			PeerId:     "peer-other",
			CreatedAt:  time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC).UnixMilli(),
			LastSeen:   time.Date(2026, 4, 12, 18, 0, 0, 0, time.UTC).UnixMilli(),
			DeviceInfo: "Windows",
		},
	}
	metadata := map[string]*account_settings.SessionPresentation{
		"peer-current": {
			PeerId:     "peer-current",
			Label:      "Chrome on macOS (Portland, OR)",
			DeviceType: "web",
			ClientName: "Chrome",
			Location:   "Portland, OR",
		},
		"peer-other": {
			PeerId:     "peer-other",
			Label:      "Alpha desktop on Windows",
			DeviceType: "desktop",
			ClientName: "Alpha desktop",
		},
	}

	got := buildCloudSessionRows("peer-current", rows, metadata)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}

	current := got[0]
	if current.GetPeerId() != "peer-current" {
		t.Fatalf("expected current peer %q, got %q", "peer-current", current.GetPeerId())
	}
	if !current.GetCurrentSession() {
		t.Fatal("expected current cloud row to be marked current")
	}
	if current.GetKind() != s4wave_account.AccountSessionKind_AccountSessionKind_ACCOUNT_SESSION_KIND_CLOUD_AUTH_SESSION {
		t.Fatalf("expected cloud session kind, got %v", current.GetKind())
	}
	if current.GetLabel() != "Chrome on macOS (Portland, OR)" {
		t.Fatalf("expected current label %q, got %q", "Chrome on macOS (Portland, OR)", current.GetLabel())
	}
	if current.GetClientName() != "Chrome" {
		t.Fatalf("expected current client name %q, got %q", "Chrome", current.GetClientName())
	}
	if current.GetLocation() != "Portland, OR" {
		t.Fatalf("expected current location %q, got %q", "Portland, OR", current.GetLocation())
	}
	if current.GetOs() != "macOS" {
		t.Fatalf("expected current os %q, got %q", "macOS", current.GetOs())
	}
	if current.GetCreatedAt() == nil || current.GetLastSeenAt() == nil {
		t.Fatal("expected current timestamps to be set")
	}

	other := got[1]
	if other.GetPeerId() != "peer-other" {
		t.Fatalf("expected other peer %q, got %q", "peer-other", other.GetPeerId())
	}
	if other.GetCurrentSession() {
		t.Fatal("expected other cloud row to be non-current")
	}
	if other.GetLabel() != "Alpha desktop on Windows" {
		t.Fatalf("expected other label %q, got %q", "Alpha desktop on Windows", other.GetLabel())
	}
	if other.GetClientName() != "Alpha desktop" {
		t.Fatalf("expected other client name %q, got %q", "Alpha desktop", other.GetClientName())
	}
	if other.GetDeviceType() != "desktop" {
		t.Fatalf("expected other device type %q, got %q", "desktop", other.GetDeviceType())
	}
	if other.GetOs() != "Windows" {
		t.Fatalf("expected other os %q, got %q", "Windows", other.GetOs())
	}
}

// TestMarshalMultiSigRequestUsesProtoBinary verifies account-resource multi-sig
// requests stay on the protobuf binary wire format expected by cloud routes.
func TestMarshalMultiSigRequestUsesProtoBinary(t *testing.T) {
	req := &api.MultiSigRequest{
		Envelope: []byte("envelope"),
		Signatures: []*api.EntitySignature{{
			PeerId:    "12D3KooWPeer",
			Signature: []byte("signature"),
			SignedAt:  timestamppb.New(time.Unix(1713571200, 123000000)),
		}},
	}

	body, err := marshalMultiSigRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var decoded api.MultiSigRequest
	if err := decoded.UnmarshalVT(body); err != nil {
		t.Fatalf("expected protobuf binary multi-sig request, got decode error: %v", err)
	}
	if !decoded.EqualVT(req) {
		t.Fatal("decoded multi-sig request does not match original")
	}
}

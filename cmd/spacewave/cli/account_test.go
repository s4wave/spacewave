//go:build !js

package spacewave_cli

import (
	"testing"

	provider_pb "github.com/s4wave/spacewave/core/provider"
	session_pb "github.com/s4wave/spacewave/core/session"
)

func TestPrintSessionListEntryShowsFollowupSessionIndex(t *testing.T) {
	// Build a follow-up session list entry.
	entry := &session_pb.SessionListEntry{
		SessionIndex: 7,
		SessionRef: &session_pb.SessionRef{
			ProviderResourceRef: &provider_pb.ProviderResourceRef{
				Id:                "session-7",
				ProviderId:        "local",
				ProviderAccountId: "account-7",
			},
		},
	}

	// Capture the printed session entry.
	out, err := captureStdout(t, func() error {
		return printSessionListEntry(entry, "text")
	})
	if err != nil {
		t.Fatalf("print session list entry: %v", err)
	}

	// Assert the follow-up session index is printed.
	assertContains(t, out, "Session Index")
	assertContains(t, out, "7")
	assertContains(t, out, "Use --session-index 7 with follow-up commands to use this session.")
}

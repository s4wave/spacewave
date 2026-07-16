package sobject_test

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestSOReceiptSignatureContexts(t *testing.T) {
	const (
		sharedObjectID    = "shared-object"
		participantPeerID = "participant-peer"
		localID           = "mutation-local"
	)

	contexts := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "lookup",
			got:  sobject.BuildSOReceiptLookupSignatureContext(sharedObjectID, participantPeerID, localID),
			want: "sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1.participant_receipt_lookup_signature shared-object participant participant-peer local-id mutation-local",
		},
		{
			name: "acknowledgement",
			got:  sobject.BuildSOReceiptAcknowledgementSignatureContext(sharedObjectID, participantPeerID, localID),
			want: "sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1.participant_receipt_acknowledgement_signature shared-object participant participant-peer local-id mutation-local",
		},
		{
			name: "terminal",
			got:  sobject.BuildSOTerminalReceiptSignatureContext(sharedObjectID, participantPeerID, localID, 7),
			want: "sobject 2024-05-22T20:10:42.613604Z shared object crypto ctx v1.validator_terminal_receipt_signature shared-object participant participant-peer local-id mutation-local root-seqno 7",
		},
	}

	seen := make(map[string]string, len(contexts))
	for _, context := range contexts {
		t.Run(context.name, func(t *testing.T) {
			if context.got != context.want {
				t.Fatalf("context = %q, want %q", context.got, context.want)
			}
		})
		if previous, ok := seen[context.got]; ok {
			t.Fatalf("context %q is not domain-separated from %s", context.name, previous)
		}
		seen[context.got] = context.name
	}
}

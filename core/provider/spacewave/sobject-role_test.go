package provider_spacewave

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestMaxSOParticipantRolePreservesHigherRole(t *testing.T) {
	tests := []struct {
		name string
		a    sobject.SOParticipantRole
		b    sobject.SOParticipantRole
		want sobject.SOParticipantRole
	}{
		{
			name: "preserve owner over writer",
			a:    sobject.SOParticipantRole_SOParticipantRole_OWNER,
			b:    sobject.SOParticipantRole_SOParticipantRole_WRITER,
			want: sobject.SOParticipantRole_SOParticipantRole_OWNER,
		},
		{
			name: "promote reader to writer",
			a:    sobject.SOParticipantRole_SOParticipantRole_READER,
			b:    sobject.SOParticipantRole_SOParticipantRole_WRITER,
			want: sobject.SOParticipantRole_SOParticipantRole_WRITER,
		},
		{
			name: "preserve validator over reader",
			a:    sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			b:    sobject.SOParticipantRole_SOParticipantRole_READER,
			want: sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maxSOParticipantRole(tc.a, tc.b); got != tc.want {
				t.Fatalf("maxSOParticipantRole() = %v, want %v", got, tc.want)
			}
		})
	}
}

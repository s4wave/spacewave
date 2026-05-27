package sobject

import "testing"

func TestMaxSOParticipantRolePreservesHigherRole(t *testing.T) {
	tests := []struct {
		name string
		a    SOParticipantRole
		b    SOParticipantRole
		want SOParticipantRole
	}{
		{
			name: "preserve owner over writer",
			a:    SOParticipantRole_SOParticipantRole_OWNER,
			b:    SOParticipantRole_SOParticipantRole_WRITER,
			want: SOParticipantRole_SOParticipantRole_OWNER,
		},
		{
			name: "promote reader to writer",
			a:    SOParticipantRole_SOParticipantRole_READER,
			b:    SOParticipantRole_SOParticipantRole_WRITER,
			want: SOParticipantRole_SOParticipantRole_WRITER,
		},
		{
			name: "preserve validator over reader",
			a:    SOParticipantRole_SOParticipantRole_VALIDATOR,
			b:    SOParticipantRole_SOParticipantRole_READER,
			want: SOParticipantRole_SOParticipantRole_VALIDATOR,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaxSOParticipantRole(tc.a, tc.b); got != tc.want {
				t.Fatalf("MaxSOParticipantRole() = %v, want %v", got, tc.want)
			}
		})
	}
}

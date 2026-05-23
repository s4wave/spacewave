package sobject_sync

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSOSyncMessageSnapshotRoundtrip(t *testing.T) {
	msg := &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{
			Snapshot: &SOSyncSnapshot{
				SoState:   []byte("test-state-data"),
				RootSeqno: 42,
			},
		},
	}

	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	decoded := &SOSyncMessage{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	snap := decoded.GetSnapshot()
	if snap == nil {
		t.Fatal("expected snapshot body")
	}
	if snap.GetRootSeqno() != 42 {
		t.Errorf("expected seqno 42, got %d", snap.GetRootSeqno())
	}
	if string(snap.GetSoState()) != "test-state-data" {
		t.Errorf("expected test-state-data, got %s", snap.GetSoState())
	}
}

func TestSOSyncMessageOpRoundtrip(t *testing.T) {
	msg := &SOSyncMessage{
		Body: &SOSyncMessage_Op{
			Op: &SOSyncOp{
				Operation: []byte("signed-op-bytes"),
				Nonce:     7,
				PeerId:    []byte("peer-id-bytes"),
			},
		},
	}

	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	decoded := &SOSyncMessage{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	op := decoded.GetOp()
	if op == nil {
		t.Fatal("expected op body")
	}
	if op.GetNonce() != 7 {
		t.Errorf("expected nonce 7, got %d", op.GetNonce())
	}
	if string(op.GetOperation()) != "signed-op-bytes" {
		t.Errorf("expected signed-op-bytes, got %s", op.GetOperation())
	}
	if string(op.GetPeerId()) != "peer-id-bytes" {
		t.Errorf("expected peer-id-bytes, got %s", op.GetPeerId())
	}
}

func TestSOSyncMessageAckRoundtrip(t *testing.T) {
	msg := &SOSyncMessage{
		Body: &SOSyncMessage_Ack{
			Ack: &SOSyncAck{
				AckedSeqno: 99,
			},
		},
	}

	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	decoded := &SOSyncMessage{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	ack := decoded.GetAck()
	if ack == nil {
		t.Fatal("expected ack body")
	}
	if ack.GetAckedSeqno() != 99 {
		t.Errorf("expected acked_seqno 99, got %d", ack.GetAckedSeqno())
	}
}

func TestSOSyncMessageOneofDispatch(t *testing.T) {
	tests := []struct {
		name string
		msg  *SOSyncMessage
		want string
	}{
		{
			name: "snapshot",
			msg: &SOSyncMessage{
				Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{RootSeqno: 1}},
			},
			want: "snapshot",
		},
		{
			name: "op",
			msg: &SOSyncMessage{
				Body: &SOSyncMessage_Op{Op: &SOSyncOp{Nonce: 1}},
			},
			want: "op",
		},
		{
			name: "ack",
			msg: &SOSyncMessage{
				Body: &SOSyncMessage_Ack{Ack: &SOSyncAck{AckedSeqno: 1}},
			},
			want: "ack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.msg.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			decoded := &SOSyncMessage{}
			if err := decoded.UnmarshalVT(data); err != nil {
				t.Fatal(err)
			}

			var got string
			switch decoded.GetBody().(type) {
			case *SOSyncMessage_Snapshot:
				got = "snapshot"
			case *SOSyncMessage_Op:
				got = "op"
			case *SOSyncMessage_Ack:
				got = "ack"
			}
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestSyncProtocolID(t *testing.T) {
	if SyncProtocolID != "alpha/so-sync" {
		t.Errorf("expected alpha/so-sync, got %s", SyncProtocolID)
	}
}

func TestNewSOSync(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetOutput(io.Discard)
	s := NewSOSync(le, nil, "test-so-id", nil)
	if s.soID != "test-so-id" {
		t.Errorf("expected test-so-id, got %s", s.soID)
	}
}

package world_block_tx

import (
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
)

func TestContainsGCSweep(t *testing.T) {
	gcTx, err := NewTxGCSweep()
	if err != nil {
		t.Fatal(err.Error())
	}
	objectTx, err := NewTxCreateObject("object", &bucket.ObjectRef{})
	if err != nil {
		t.Fatal(err.Error())
	}
	mixedBatch, err := NewTxBatch(&TxBatch{Txs: []*Tx{objectTx, gcTx}})
	if err != nil {
		t.Fatal(err.Error())
	}
	nestedBatch, err := NewTxBatch(&TxBatch{Txs: []*Tx{objectTx, mixedBatch}})
	if err != nil {
		t.Fatal(err.Error())
	}
	ordinaryBatch, err := NewTxBatch(&TxBatch{Txs: []*Tx{objectTx, objectTx.Clone()}})
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, test := range []struct {
		name string
		tx   *Tx
		want bool
	}{
		{name: "nil"},
		{name: "empty", tx: &Tx{}},
		{name: "ordinary", tx: objectTx},
		{name: "top level sweep", tx: gcTx, want: true},
		{name: "ordinary batch", tx: ordinaryBatch},
		{name: "mixed batch", tx: mixedBatch, want: true},
		{name: "nested batch", tx: nestedBatch, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ContainsGCSweep(test.tx); got != test.want {
				t.Fatalf("ContainsGCSweep() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTxGCSweepIntentConstructors(t *testing.T) {
	for _, test := range []struct {
		name string
		ctor func() (*Tx, error)
		want TxGCSweepIntent
	}{
		{
			name: "legacy",
			ctor: NewTxGCSweep,
			want: TxGCSweepIntent_TxGCSweepIntent_LEGACY_MAINTENANCE,
		},
		{
			name: "maintenance",
			ctor: NewMaintenanceTxGCSweep,
			want: TxGCSweepIntent_TxGCSweepIntent_MAINTENANCE,
		},
		{
			name: "explicit",
			ctor: NewExplicitTxGCSweep,
			want: TxGCSweepIntent_TxGCSweepIntent_EXPLICIT,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := test.ctor()
			if err != nil {
				t.Fatal(err.Error())
			}
			if got := tx.GetTxGcSweep().GetIntent(); got != test.want {
				t.Fatalf("intent = %s, want %s", got.String(), test.want.String())
			}
			if got := tx.Clone().GetTxGcSweep().GetIntent(); got != test.want {
				t.Fatalf("cloned intent = %s, want %s", got.String(), test.want.String())
			}
			if got := tx.GetTxGcSweep().Clone().GetIntent(); got != test.want {
				t.Fatalf("cloned sweep intent = %s, want %s", got.String(), test.want.String())
			}
		})
	}
}

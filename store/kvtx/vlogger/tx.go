package store_kvtx_vlogger

import (
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/sirupsen/logrus"
)

// Tx is a verbose transaction
type Tx = kvtx_vlogger.Tx

// NewTx constructs a new transaction.
func NewTx(le *logrus.Entry, tx kvtx.Tx) *Tx {
	return kvtx_vlogger.NewTx(le, tx)
}

// _ is a type assertion
var _ kvtx.Tx = ((*Tx)(nil))

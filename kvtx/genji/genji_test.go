package kvtx_genji

import (
	"testing"

	store_kvtx_inmem "github.com/aperturerobotics/hydra/store/kvtx/inmem"
	gengine "github.com/genjidb/genji/engine"
	"github.com/genjidb/genji/engine/enginetest"
	"github.com/sirupsen/logrus"
)

// TestGenjiEngine tests the genji engine store.
func TestGenjiEngine(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	enginetest.TestSuite(t, func() (gengine.Engine, func()) {
		e := NewEngine(store_kvtx_inmem.NewStore())
		return e, func() {
			_ = e.Close()
		}
	})
}

package kvtx_vlogger

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	sinmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestVlogger(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	vstore := NewVLogger(le, underlyingStore)
	if err := kvtx_kvtest.TestAll(ctx, vstore); err != nil {
		t.Fatal(err.Error())
	}
}

func TestKeyForLoggingRedactsKeyMaterial(t *testing.T) {
	const secret = "password=correct-horse-battery-staple"

	logBuf := bytes.NewBuffer(nil)
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(logBuf)
	le := logrus.NewEntry(log)

	var underlyingStore kvtx.Store = sinmem.NewStore()
	vstore := NewVLogger(le, underlyingStore)
	tx, err := vstore.NewTransaction(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	if err := tx.Set(context.Background(), []byte(secret), []byte("value")); err != nil {
		t.Fatal(err)
	}

	output := logBuf.String()
	if strings.Contains(output, secret) {
		t.Fatalf("vlogger exposed key material in logs: %s", output)
	}
	if !strings.Contains(output, "len=") {
		t.Fatalf("vlogger did not include structural key summary: %s", output)
	}
}

package resource_world

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestTypedObjectResourceKeyString(t *testing.T) {
	if got, want := (typedObjectResourceKey{
		typeID:    "unixfs/fs-node",
		objectKey: "files",
		readOnly:  false,
	}).String(), "typed-object type=unixfs/fs-node object=files readOnly=false"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	sessionPeerID := peer.ID("session-peer")
	got := (typedObjectResourceKey{
		typeID:        "notes/doc",
		objectKey:     "docs/readme",
		readOnly:      true,
		sessionPeerID: sessionPeerID,
		engineID:      "engine-1",
	}).String()
	want := "typed-object type=notes/doc object=docs/readme readOnly=true sessionPeerID=" +
		sessionPeerID.String() + " engineID=engine-1"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestTypedObjectResourceExitLogUsesKeyName(t *testing.T) {
	var out bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&out)
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})

	cb := keyed.NewLogExitedCallbackWithNameFn[typedObjectResourceKey, *typedObjectHandle](
		logrus.NewEntry(logger),
		typedObjectResourceKey.String,
	)
	cb(typedObjectResourceKey{
		typeID:    "unixfs/fs-node",
		objectKey: "files",
		readOnly:  false,
	}, nil, nil, nil)

	got := out.String()
	if !strings.Contains(got, "keyed: routine exited: typed-object type=unixfs/fs-node object=files readOnly=false") {
		t.Fatalf("exit log = %q", got)
	}
	if strings.Contains(got, "_fields") || strings.Contains(got, "__goPointer") {
		t.Fatalf("exit log leaked GoScript struct internals: %q", got)
	}
}

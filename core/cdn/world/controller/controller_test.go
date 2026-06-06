//go:build js || goscript

package cdn_world_controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	"github.com/sirupsen/logrus"
)

const testSpaceID = "01kpftest0000000000000001"

func TestExecuteWaitsForMissingPublishedHead(t *testing.T) {
	ptrBytes, err := (&alpha_cdn.CdnRootPointer{
		SpaceId: testSpaceID,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	encoded := []byte(packedmsg.EncodePackedMessage(ptrBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/"+testSpaceID+"/root.packedmsg", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(encoded)
	})
	hs := httptest.NewServer(mux)
	defer hs.Close()

	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, NewConfig("release-world", testSpaceID, hs.URL))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ctrl.Execute(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("Execute exited while CDN head was still missing: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer waitCancel()
	if eng, err := ctrl.GetWorldEngine(waitCtx); err == nil {
		t.Fatalf("GetWorldEngine returned engine before CDN head existed: %v", eng)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not stop after context cancel")
	}
}

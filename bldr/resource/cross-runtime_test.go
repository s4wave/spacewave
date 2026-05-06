package resource_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

func TestTypeScriptAttachedResourceTreePublishesCallableChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootMux := srpc.NewMux()
	server := resource_server.NewResourceServer(rootMux)
	resourceMux := srpc.NewMux()
	if err := server.Register(resourceMux); err != nil {
		t.Fatalf("register resource server: %v", err)
	}
	srpcServer := srpc.NewServer(resourceMux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go srpcServer.HandleStream(ctx, conn)
		}
	}()

	cmd := exec.CommandContext(ctx, "bun", "run", "./testdata/cross-runtime-ts-client.ts", ln.Addr().String())
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("typescript cross-runtime client failed: %v\n%s", err, out)
	}
}

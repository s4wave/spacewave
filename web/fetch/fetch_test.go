package web_fetch

import (
	"bytes"
	context "context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

func TestFetch(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// this is a valid hello-world.wasm.br file
	testWasmBrData := []byte{0xa1, 0xb0, 0x1, 0xc0, 0x2f, 0xf0, 0xef, 0xb6, 0xde, 0xdf, 0xd0, 0xa1, 0x16, 0x43, 0x34, 0x47, 0x5, 0x93, 0x70, 0xe9, 0xe8, 0x4b, 0x4d, 0x70, 0x21, 0xa9, 0xc, 0x48, 0x65, 0xe1, 0xfc, 0x9f, 0x0, 0x85, 0xb6, 0x65, 0x2a, 0xdd, 0x44, 0x71, 0x41, 0x4c, 0xf3, 0x73, 0x2f, 0xd4, 0x8a, 0xd1, 0x9b, 0x82, 0x85, 0xde, 0x0}
	testWasmFilename := "hello-world.wasm.br"

	fetchServer := NewFetchServer(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != testWasmFilename {
			rw.WriteHeader(404)
			return
		}

		rw.Header().Add("Content-Type", "application/wasm")
		rw.Header().Add("Content-Encoding", "br")
		rw.WriteHeader(200)
		_, err := io.Copy(rw, bytes.NewReader(testWasmBrData))
		if err != nil {
			le.Fatalf(err.Error())
		}
	})

	// create the srpc bus for the server
	serverMux := srpc.NewMux()
	_ = SRPCRegisterFetchService(serverMux, fetchServer)

	// create the srpc server
	srpcServer := srpc.NewServer(serverMux)

	// create the srpc client
	openStream := srpc.NewServerPipe(srpcServer)
	client := srpc.NewClient(openStream)
	fetchClient := NewSRPCFetchServiceClient(client)

	// test the mime type of a .wasm.br file
	req, err := http.NewRequest("GET", testWasmFilename, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	rw := httptest.NewRecorder()
	if err := Fetch(ctx, fetchClient.Fetch, req, rw); err != nil {
		t.Fatal(err.Error())
	}

	res := rw.Result()
	if res.StatusCode != 200 {
		t.Fatalf("status code: %d", res.StatusCode)
	}
	readData, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(readData, testWasmBrData) {
		t.Fatalf("read data does not match test data: %#v", string(readData))
	}
	if contentType := res.Header.Get("content-type"); contentType != "application/wasm" {
		t.Fatalf("incorrect content type: %s", contentType)
	}
	if contentEnc := res.Header.Get("content-encoding"); contentEnc != "br" {
		t.Fatalf("incorrect content encoding: %s", contentEnc)
	}
}

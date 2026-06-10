package web_fetch

import (
	"bytes"
	context "context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
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
			le.Fatal(err.Error())
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

// newFetchTestClient builds a SRPC pipe client over the given fetch server.
func newFetchTestClient(server SRPCFetchServiceServer) SRPCFetchServiceClient {
	serverMux := srpc.NewMux()
	_ = SRPCRegisterFetchService(serverMux, server)
	srpcServer := srpc.NewServer(serverMux)
	openStream := srpc.NewServerPipe(srpcServer)
	return NewSRPCFetchServiceClient(srpc.NewClient(openStream))
}

// truncatingFetchServer sends response headers and a partial body packet,
// then ends the stream without the final done packet.
type truncatingFetchServer struct{}

func (s *truncatingFetchServer) Fetch(strm SRPCFetchService_FetchStream) error {
	if _, err := strm.Recv(); err != nil {
		return err
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "text/javascript")
	if err := strm.Send(BuildFetchResponse_Info(hdr, 200)); err != nil {
		return err
	}
	return strm.Send(BuildFetchResponse_Data([]byte("partial"), false))
}

func TestFetchStreamEOFBeforeDoneIsError(t *testing.T) {
	ctx := context.Background()
	fetchClient := newFetchTestClient(&truncatingFetchServer{})

	req, err := http.NewRequest("GET", "/module.mjs", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rw := httptest.NewRecorder()
	err = Fetch(ctx, fetchClient.Fetch, req, rw)
	if err == nil {
		t.Fatal("expected error for stream EOF before the final done packet")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF, got: %v", err)
	}
}

func TestHandleFetchContentLengthMismatchIsError(t *testing.T) {
	ctx := context.Background()
	fetchClient := newFetchTestClient(NewFetchServer(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Length", "100")
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte("short body"))
	}))

	req, err := http.NewRequest("GET", "/module.mjs", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rw := httptest.NewRecorder()
	err = Fetch(ctx, fetchClient.Fetch, req, rw)
	if err == nil {
		t.Fatal("expected error for body shorter than declared Content-Length")
	}
}

func TestHandleFetchHeadIgnoresContentLength(t *testing.T) {
	ctx := context.Background()
	fetchClient := newFetchTestClient(NewFetchServer(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Length", "100")
		rw.WriteHeader(200)
	}))

	req, err := http.NewRequest("HEAD", "/module.mjs", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rw := httptest.NewRecorder()
	if err := Fetch(ctx, fetchClient.Fetch, req, rw); err != nil {
		t.Fatalf("expected HEAD with Content-Length to succeed: %v", err)
	}
}

func TestHandleFetchAbortedBodyIsError(t *testing.T) {
	ctx := context.Background()
	fetchClient := newFetchTestClient(NewFetchServer(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte("partial"))
		rw.(BodyAborter).Abort(errors.New("upstream body failed"))
	}))

	req, err := http.NewRequest("GET", "/module.mjs", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rw := httptest.NewRecorder()
	err = Fetch(ctx, fetchClient.Fetch, req, rw)
	if err == nil {
		t.Fatal("expected error for aborted response body")
	}
}

func TestHandleFetchHandlerPanicIsError(t *testing.T) {
	ctx := context.Background()
	fetchClient := newFetchTestClient(NewFetchServer(func(rw http.ResponseWriter, req *http.Request) {
		panic("handler exploded")
	}))

	req, err := http.NewRequest("GET", "/module.mjs", nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rw := httptest.NewRecorder()
	err = Fetch(ctx, fetchClient.Fetch, req, rw)
	if err == nil {
		t.Fatal("expected error for handler panic")
	}
	if !strings.Contains(err.Error(), "handler exploded") {
		t.Fatalf("expected panic message in error, got: %v", err)
	}
}

func TestFetchDataPacketsOwnPayloadBytes(t *testing.T) {
	reqData := []byte("request-payload")
	reqPkt := NewFetchRequestWithData(reqData, false)
	reqData[0] = 'X'
	if got, want := reqPkt.GetRequestData().GetData(), []byte("request-payload"); !bytes.Equal(got, want) {
		t.Fatalf("request packet data mutated: got %q, want %q", got, want)
	}

	respData := []byte("response-payload")
	respPkt := BuildFetchResponse_Data(respData, false)
	respData[0] = 'X'
	if got, want := respPkt.GetResponseData().GetData(), []byte("response-payload"); !bytes.Equal(got, want) {
		t.Fatalf("response packet data mutated: got %q, want %q", got, want)
	}
}

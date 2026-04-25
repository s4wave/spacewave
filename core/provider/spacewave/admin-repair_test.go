package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestApplyPackMetadataRepair(t *testing.T) {
	var gotReq api.PackMetadataRepairRequest
	respData, err := (&api.PackMetadataRepairResponse{
		Scanned: 1,
		Changed: 1,
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/admin/bstore/01kny7hn4wp25f7t86xzww6bd6/pack-metadata-repair" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := gotReq.UnmarshalVT(readRequestBody(t, r)); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respData)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(
		http.DefaultClient,
		srv.URL,
		DefaultSigningEnvPrefix,
		priv,
		pid.String(),
	)

	resp, err := cli.ApplyPackMetadataRepair(context.Background(), "01kny7hn4wp25f7t86xzww6bd6", &api.PackMetadataRepairRequest{
		Entries: []*api.PackMetadataRepairEntry{{
			Id:          "01kny7hn7r5qzaznnsvpqf7p2m",
			BloomFilter: []byte{1, 2, 3},
			BlockCount:  2,
			SizeBytes:   4,
			Sha256Hex:   "sha",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyPackMetadataRepair: %v", err)
	}
	if resp.GetScanned() != 1 || resp.GetChanged() != 1 {
		t.Fatalf("response = %#v", resp)
	}
	if len(gotReq.GetEntries()) != 1 || gotReq.GetEntries()[0].GetId() != "01kny7hn7r5qzaznnsvpqf7p2m" {
		t.Fatalf("unexpected request: %#v", gotReq.GetEntries())
	}
}

func readRequestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

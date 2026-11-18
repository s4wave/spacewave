package world

import (
	"context"
	"testing"
)

func TestGetObjectBodiesBatchPageWithholdsOversizedBody(t *testing.T) {
	// Prepare a body whose encoded entry alone exceeds the page budget.
	const byteBudget = 100
	body := make([]byte, byteBudget)
	keys := []string{"body/oversized", "body/small"}

	// Request a page holding the oversized body and a small one after it.
	page, consumed, err := getObjectBodiesBatchPage(
		context.Background(),
		keys,
		byteBudget,
		func(_ context.Context, key string) (*ObjectBody, error) {
			if key == "body/small" {
				return &ObjectBody{ObjectKey: key, Body: []byte("small"), Rev: 2, Exists: true}, nil
			}
			return &ObjectBody{ObjectKey: key, Body: body, Rev: 1, Exists: true}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the oversized entry withheld with its size and the page to go on.
	if len(page) != 2 || consumed != 0 {
		t.Fatalf("page = %v, consumed = %d, want both keys answered", page, consumed)
	}
	withheld := page[0]
	if withheld.ObjectKey != keys[0] || !withheld.Exists || withheld.Rev != 1 || withheld.Body != nil || withheld.WithheldSize != byteBudget {
		t.Fatalf("withheld entry = %+v", withheld)
	}
	if string(page[1].Body) != "small" || page[1].WithheldSize != 0 {
		t.Fatalf("small entry = %+v", page[1])
	}
}

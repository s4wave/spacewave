package world

import (
	"context"
	"errors"
	"testing"
)

func TestGetObjectBodiesBatchPageRejectsOversizedFirstBody(t *testing.T) {
	// Prepare an object body whose encoded response exceeds the page budget.
	const byteBudget = 100
	key := "body/oversized"
	body := make([]byte, byteBudget)

	// Request a page containing the oversized first object body.
	page, consumed, err := getObjectBodiesBatchPage(
		context.Background(),
		[]string{key},
		byteBudget,
		func(context.Context, string) ([]byte, uint64, bool, error) {
			return body, 1, true, nil
		},
	)

	// Require the oversized key error and an empty response page.
	var tooLargeErr *ObjectBodyTooLargeError
	if !errors.As(err, &tooLargeErr) {
		t.Fatalf("error = %v, want ObjectBodyTooLargeError", err)
	}
	if tooLargeErr.ObjectKey != key {
		t.Fatalf("oversized key = %q, want %q", tooLargeErr.ObjectKey, key)
	}
	if page != nil || consumed != 0 {
		t.Fatalf("page = %v, consumed = %d, want no response page", page, consumed)
	}
}

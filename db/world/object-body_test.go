package world

import (
	"context"
	"errors"
	"testing"
)

func TestGetObjectBodiesBatchPageRejectsOversizedFirstBody(t *testing.T) {
	const byteBudget = 100
	key := "body/oversized"
	body := make([]byte, byteBudget)

	page, consumed, err := getObjectBodiesBatchPage(
		context.Background(),
		[]string{key},
		byteBudget,
		func(context.Context, string) ([]byte, bool, error) {
			return body, true, nil
		},
	)
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

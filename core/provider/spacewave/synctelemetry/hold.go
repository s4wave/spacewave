package synctelemetry

import (
	"errors"

	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
)

// UploadHold names the account limit that holds a store's uploads.
type UploadHold int

const (
	// UploadHoldNone means no account limit holds the uploads.
	UploadHoldNone UploadHold = iota
	// UploadHoldStorageFull means the cloud refused an upload because the
	// account's storage is full.
	UploadHoldStorageFull
	// UploadHoldSpendingLimit means the cloud refused an upload because the
	// account reached its spending limit.
	UploadHoldSpendingLimit
)

// uploadHoldOf returns the account limit that err reports, if any.
func uploadHoldOf(err error) UploadHold {
	var ce *clouderror.Error
	if !errors.As(err, &ce) {
		return UploadHoldNone
	}
	switch ce.Code {
	case "storage_limit":
		return UploadHoldStorageFull
	case "spending_limit":
		return UploadHoldSpendingLimit
	}
	return UploadHoldNone
}

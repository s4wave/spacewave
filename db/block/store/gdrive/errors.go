// Package block_store_gdrive stores immutable ciphertext objects in Google Drive.
// It never deletes or updates an existing file, including an orphaned upload.
package block_store_gdrive

import "github.com/pkg/errors"

var (
	// ErrUploadLimit asks the caller to wait for the daily upload allowance.
	ErrUploadLimit = errors.New("Drive daily upload limit reached")
	// ErrQuotaFull asks the owner to free space or increase their Drive quota.
	ErrQuotaFull = errors.New("Drive storage quota is full")
	// ErrSignIn asks the owner to sign in again after authorization is revoked.
	ErrSignIn = errors.New("Drive authorization expired; sign in again")
	// ErrDeleteForbidden prevents a backup from propagating a local deletion.
	ErrDeleteForbidden = errors.New("Drive backup objects cannot be deleted")
	// ErrImmutable prevents replacing a key with different bytes.
	ErrImmutable = errors.New("Drive backup object already holds different bytes")
)

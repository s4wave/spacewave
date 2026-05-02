package pagestore

import "github.com/pkg/errors"

// CorruptPageError identifies a referenced page that cannot be decoded safely.
type CorruptPageError struct {
	PageID PageID
	Err    error
}

// NewCorruptPageError constructs a corrupt page error for pageID.
func NewCorruptPageError(pageID PageID, err error) *CorruptPageError {
	return &CorruptPageError{
		PageID: pageID,
		Err:    err,
	}
}

// Error returns the corruption error message.
func (e *CorruptPageError) Error() string {
	if e.Err == nil {
		return errors.Errorf("corrupt page %d", e.PageID).Error()
	}
	return errors.Wrapf(e.Err, "corrupt page %d", e.PageID).Error()
}

// Unwrap returns the underlying page decode error.
func (e *CorruptPageError) Unwrap() error {
	return e.Err
}

// IsCorruptPageError reports whether err includes a corrupt page error.
func IsCorruptPageError(err error) bool {
	var pageErr *CorruptPageError
	return errors.As(err, &pageErr)
}

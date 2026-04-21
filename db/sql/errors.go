package sql

import "errors"

// ErrEmptySqlDbId is returned if the sql db id is empty.
var ErrEmptySqlDbId = errors.New("empty sql db id")

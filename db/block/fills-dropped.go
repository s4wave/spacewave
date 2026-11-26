package block

import (
	"strconv"
	"strings"
)

// FillsDroppedError is the result of a batch of cache fills of which a Volume
// dropped some, as ErrRefsUnknown is the result of one dropped fill: the
// entries at At stored nothing, and every other entry was stored, with the
// batch's existence results valid for each. It matches ErrRefsUnknown, so a
// caller that does not split the batch takes the whole batch as not stored,
// which a later read redoes.
type FillsDroppedError struct {
	// At are the dropped entries' indexes in the batch, ascending.
	At []int
}

// Error names the dropped entries.
func (e *FillsDroppedError) Error() string {
	// Follow the refs-unknown message with each dropped index.
	var b strings.Builder
	b.WriteString(ErrRefsUnknown.Error())
	b.WriteString(": dropped cache fills at")
	for _, i := range e.At {
		b.WriteByte(' ')
		b.WriteString(strconv.Itoa(i))
	}
	return b.String()
}

// Is matches ErrRefsUnknown.
func (e *FillsDroppedError) Is(target error) bool {
	return target == ErrRefsUnknown
}

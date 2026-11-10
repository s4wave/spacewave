//go:build !linux

package common

import "context"

// punchStep does nothing off Linux, which alone has fallocate(PUNCH_HOLE).
func (s *Store[T]) punchStep(context.Context) {}

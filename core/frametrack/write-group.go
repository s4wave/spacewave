package frametrack

import "context"

// WriteGroup opens a group and calls drain to deliver its frames in order.
// drain runs until the group ends or the frame writer returns an error.
// WriteGroup releases its writer when ctx ends, including a blocked write.
type WriteGroup func(ctx context.Context, header *GroupHeader, drain func(func(*Frame) error) error) error

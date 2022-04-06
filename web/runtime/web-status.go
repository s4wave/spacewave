package web_runtime

// Prefix is the prefix used for messaging.
var Prefix = "bldr/"

// NewQueryWebStatus constructs a new message to query web runtime status.
func NewQueryWebStatus() *RuntimeToWeb {
	return &RuntimeToWeb{
		MessageType:     RuntimeToWebType_RuntimeToWebType_QUERY_STATUS,
		QueryViewStatus: &QueryWebStatus{},
	}
}

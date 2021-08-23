package web

// Prefix is the prefix used for messaging.
var Prefix = "@aperturerobotics/bldr"

// NewQueryViewStatus constructs a new message to query webview status.
func NewQueryViewStatus() *RuntimeToWeb {
	return &RuntimeToWeb{
		MessageType:     RuntimeToWebType_RuntimeToWebType_QUERY_STATUS,
		QueryViewStatus: &QueryWebStatus{},
	}
}

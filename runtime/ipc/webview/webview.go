package webview

// NewQueryViewStatus constructs a new message to query webview status.
func NewQueryViewStatus() *RuntimeToWebView {
	return &RuntimeToWebView{
		MessageType:     RuntimeToWebViewType_RuntimeToWebViewType_QUERY_STATUS,
		QueryViewStatus: &QueryViewStatus{},
	}
}

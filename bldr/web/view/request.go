package web_view

import "github.com/sirupsen/logrus"

// Logger builds the logger for the request.
func (m *SetRenderModeRequest) Logger(le *logrus.Entry) *logrus.Entry {
	fields := logrus.Fields{
		"render-mode": m.GetRenderMode().String(),
	}
	if p := m.GetScriptPath(); p != "" {
		fields["script-path"] = p
	}
	return le.WithFields(fields)
}

// Logger builds the logger for the request.
func (m *SetHtmlLinksRequest) Logger(le *logrus.Entry) *logrus.Entry {
	// Describe the request's clearing and removal operations in the log fields.
	fields := logrus.Fields{}
	if m.GetClear() {
		fields["clear"] = true
	}
	if remove := m.GetRemove(); len(remove) != 0 {
		fields["remove"] = remove
	}

	// Describe each replacement link by its relation and destination.
	for id, link := range m.GetSetLinks() {
		fields["set-"+id] = link.GetRel() + "@" + link.GetHref()
	}

	return le.WithFields(fields)
}

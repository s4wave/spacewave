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
	fields := logrus.Fields{}
	if m.GetClear() {
		fields["clear"] = true
	}
	if remove := m.GetRemove(); len(remove) != 0 {
		fields["remove"] = remove
	}
	for id, link := range m.GetSetLinks() {
		fields["set-"+id] = link.GetRel() + "@" + link.GetHref()
	}
	return le.WithFields(fields)
}

package web_view

import "github.com/sirupsen/logrus"

// Logger builds the logger for the request.
func (r *SetRenderModeRequest) Logger(le *logrus.Entry) *logrus.Entry {
	fields := logrus.Fields{
		"render-mode": r.GetRenderMode().String(),
	}
	if p := r.GetScriptPath(); p != "" {
		fields["script-path"] = p
	}
	return le.WithFields(fields)
}

// Logger builds the logger for the request.
func (r *SetHtmlLinksRequest) Logger(le *logrus.Entry) *logrus.Entry {
	fields := logrus.Fields{}
	if r.GetClear() {
		fields["clear"] = true
	}
	if remove := r.GetRemove(); len(remove) != 0 {
		fields["remove"] = remove
	}
	for id, link := range r.GetSetLinks() {
		fields["set-"+id] = link.GetRel() + "@" + link.GetHref()
	}
	return le.WithFields(fields)
}

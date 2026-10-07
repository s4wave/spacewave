package world

// graphPattern is a graph quad filter. Empty fields match any value.
type graphPattern struct {
	subject, predicate, obj, label string
}

// matches checks whether q matches every non-empty field of the pattern.
func (p graphPattern) matches(q GraphQuad) bool {
	return matchesField(p.subject, q.GetSubject()) &&
		matchesField(p.predicate, q.GetPredicate()) &&
		matchesField(p.obj, q.GetObj()) &&
		matchesField(p.label, q.GetLabel())
}

// matchesField checks one pattern field. An empty pattern matches any value.
func matchesField(pattern, value string) bool {
	return pattern == "" || pattern == value
}

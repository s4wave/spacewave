package world

import "github.com/cayleygraph/quad"

// GraphQuad is the common graph entry interface.
type GraphQuad interface {
	// GetSubject returns the subject field.
	GetSubject() string
	// GetPredicate returns the predicate field.
	GetPredicate() string
	// GetObject returns the object field.
	GetObject() string
	// GetValue returns the value field.
	// (empty in most cases)
	GetValue() string
}

// GraphQuadStringToCayleyValue converts a graph quad string to a quad.Value
func GraphQuadStringToCayleyValue(s string) quad.Value {
	// note: this checks the first few characters for < or _: or @
	return quad.StringToValue(s)
}

// GraphQuadToCayleyQuad converts a graph quad to a cayley quad.
func GraphQuadToCayleyQuad(q GraphQuad, check bool) (quad.Quad, error) {
	oq := quad.Quad{}
	if q == nil {
		return oq, ErrNilQuad
	}
	if check {
		if s := q.GetSubject(); len(s) == 0 {
			return oq, ErrEmptyQuadSubject
		}
		if p := q.GetPredicate(); len(p) == 0 {
			return oq, ErrEmptyQuadPred
		}
		if o := q.GetObject(); len(o) == 0 {
			return oq, ErrEmptyQuadObject
		}
	}
	oq.Subject = GraphQuadStringToCayleyValue(q.GetSubject())
	oq.Predicate = GraphQuadStringToCayleyValue(q.GetPredicate())
	oq.Object = GraphQuadStringToCayleyValue(q.GetObject())
	oq.Label = GraphQuadStringToCayleyValue(q.GetValue())
	var err error
	if check {
		err = ValidateCayleyQuad(oq)
	}
	return oq, err
}

// ValidateGraphQuad checks a graph quad for validity.
func ValidateGraphQuad(q GraphQuad) error {
	_, err := GraphQuadToCayleyQuad(q, true)
	return err
}

// ValidateCayleyQuad checks a cayley quad for validity.
func ValidateCayleyQuad(q quad.Quad) error {
	if q.Subject == nil {
		return ErrEmptyQuadSubject
	}
	if q.Predicate == nil {
		return ErrEmptyQuadPred
	}
	if q.Object == nil {
		return ErrEmptyQuadObject
	}
	// subject must be iri
	if _, ok := q.Subject.(quad.IRI); !ok {
		return ErrQuadSubjectNotIRI
	}
	if _, ok := q.Object.(quad.IRI); !ok {
		return ErrQuadObjectNotIRI
	}
	return nil
}

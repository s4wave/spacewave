package world

import (
	"github.com/aperturerobotics/cayley/quad"
	bquad "github.com/s4wave/spacewave/db/block/quad"
)

// GraphQuad is the common graph entry interface.
type GraphQuad interface {
	// GetSubject returns the subject field.
	GetSubject() string
	// GetPredicate returns the predicate field.
	GetPredicate() string
	// GetObj returns the object field.
	GetObj() string
	// GetLabel returns the label field.
	// (empty in most cases)
	GetLabel() string
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
		if o := q.GetObj(); len(o) == 0 {
			return oq, ErrEmptyQuadObject
		}
	}
	oq.Subject = GraphQuadStringToCayleyValue(q.GetSubject())
	oq.Predicate = GraphQuadStringToCayleyValue(q.GetPredicate())
	oq.Object = GraphQuadStringToCayleyValue(q.GetObj())
	oq.Label = GraphQuadStringToCayleyValue(q.GetLabel())
	var err error
	if check {
		err = ValidateCayleyQuad(oq)
	}
	return oq, err
}

// CayleyQuadToGraphQuad converts a cayley quad into a graph quad.
func CayleyQuadToGraphQuad(q quad.Quad) GraphQuad {
	var subj, pred, obj, label string
	if q.Subject != nil {
		subj = q.Subject.String()
	}
	if q.Predicate != nil {
		pred = q.Predicate.String()
	}
	if q.Object != nil {
		obj = q.Object.String()
	}
	if q.Label != nil {
		label = q.Label.String()
	}
	return NewGraphQuad(subj, pred, obj, label)
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

// graphQuad implements GraphQuad with a struct.
type graphQuad struct {
	subj, pred, obj, value string
}

// GraphQuadToQuad constructs a new Quad object.
func GraphQuadToQuad(gq GraphQuad) *bquad.Quad {
	return &bquad.Quad{
		Subject:   gq.GetSubject(),
		Predicate: gq.GetPredicate(),
		Obj:       gq.GetObj(),
		Label:     gq.GetLabel(),
	}
}

// QuadToGraphQuad converts quad into a graph quad.
func QuadToGraphQuad(q *bquad.Quad) GraphQuad {
	return NewGraphQuad(
		q.GetSubject(),
		q.GetPredicate(),
		q.GetObj(),
		q.GetLabel(),
	)
}

// NewGraphQuad constructs a new in-memory GraphQuad.
func NewGraphQuad(subj, pred, obj, value string) GraphQuad {
	return &graphQuad{
		subj:  subj,
		pred:  pred,
		obj:   obj,
		value: value,
	}
}

// NewGraphQuadWithKeys creates a new graph quad from object keys.
func NewGraphQuadWithKeys(subjKey, pred, objKey, value string) GraphQuad {
	var subj, obj string
	if subjKey != "" {
		subj = quad.IRI(subjKey).String()
	}
	if objKey != "" {
		obj = quad.IRI(objKey).String()
	}
	return NewGraphQuad(subj, pred, obj, value)
}

// GetSubject returns the subject field.
func (g *graphQuad) GetSubject() string {
	return g.subj
}

// GetPredicate returns the predicate field.
func (g *graphQuad) GetPredicate() string {
	return g.pred
}

// GetObj returns the object field.
func (g *graphQuad) GetObj() string {
	return g.obj
}

// GetLabel returns the value field.
// (empty in most cases)
func (g *graphQuad) GetLabel() string {
	return g.value
}

// _ is a type assertion
var _ GraphQuad = ((*graphQuad)(nil))

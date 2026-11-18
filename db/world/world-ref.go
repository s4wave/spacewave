package world

import (
	"context"
	"time"

	"github.com/aperturerobotics/cayley/quad"
)

// RefRecord is a record whose proto declares World references with the
// (world.ref) field option. The aptre Go generator implements it.
type RefRecord interface {
	// RangeWorldRefs calls cb with each declared reference, including those of
	// nested message fields, until cb returns false.
	// predicate is the bare IRI the option declares.
	// target is the referenced object key and is never empty.
	// indexed is set when the option asks for a graph quad.
	//
	// Returns false if cb stopped the walk.
	RangeWorldRefs(cb func(predicate, target string, indexed bool) bool) bool
}

// SpanRecord is a record whose proto declares its time span with the
// (world.span) message option. The aptre Go generator implements it.
type SpanRecord interface {
	// WorldSpan returns the declared start and end times.
	// An unset time is zero.
	// Returns an error if a string time is not RFC 3339.
	WorldSpan() (start, end time.Time, err error)
}

// SetRefQuads writes each indexed reference of rec as a graph quad from
// subject with an empty label.
// Call it in the transaction that writes the record at object key subject,
// after creating subject and every indexed target: the graph refuses a quad
// whose endpoint does not exist, so a missing target fails the write.
// Writing an existing quad again leaves the graph unchanged.
func SetRefQuads(ctx context.Context, ws WorldStateGraph, subject string, rec RefRecord) error {
	return rangeRefQuads(rec, subject, func(q GraphQuad) error {
		return ws.SetGraphQuad(ctx, q)
	})
}

// DeleteRefQuads deletes each indexed reference quad of rec from subject.
// Call it in the transaction that removes or replaces the record at object
// key subject. DeleteObject already removes every quad of a deleted object.
func DeleteRefQuads(ctx context.Context, ws WorldStateGraph, subject string, rec RefRecord) error {
	return rangeRefQuads(rec, subject, func(q GraphQuad) error {
		return ws.DeleteGraphQuad(ctx, q)
	})
}

// rangeRefQuads calls cb with the quad of each indexed reference of rec from
// subject, until cb returns an error. It allocates one GraphQuad per indexed
// reference.
func rangeRefQuads(rec RefRecord, subject string, cb func(q GraphQuad) error) error {
	var err error
	rec.RangeWorldRefs(func(predicate, target string, indexed bool) bool {
		if !indexed {
			return true
		}
		err = cb(NewGraphQuadWithKeys(subject, quad.IRI(predicate).String(), target, ""))
		return err == nil
	})
	return err
}

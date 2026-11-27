// Package keyedview tells a reader what changed in a keyed view since the
// version it holds.
//
// A keyed view is a result made of records with stable keys, such as
// playlists by ID. Each result gets a version named by its content: the
// digest of every record's key and encoding. A reader that supplies the version
// it holds learns which keys changed and which went away, so it receives only
// those records. A reader whose version is unknown, because the reader never
// read the view or the History forgot the version, receives every record once.
// A view also has a scope, such as the map its records belong to. A reader
// whose version belongs to another scope receives every record once too, since
// nothing it holds carries over.
package keyedview

// Record is a record of a keyed view.
type Record interface {
	// MarshalVT encodes the record. Equal records encode equally.
	MarshalVT() ([]byte, error)
}

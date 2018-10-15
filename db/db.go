package db

import (
	"github.com/cayleygraph/cayley/graph"
	// _ imports the boltdb
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/sirupsen/logrus"
)

// Database implements the hydra database.
type Database struct {
	// Handle is the graph database
	*graph.Handle
	// le is the logger
	le *logrus.Entry
}

// NewDatabase builds the database with a graph handle
func NewDatabase(graphHandle *graph.Handle, le *logrus.Entry) (*Database, error) {
	return &Database{Handle: graphHandle, le: le}, nil
}

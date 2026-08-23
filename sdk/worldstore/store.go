// Package worldstore provides typed access to world-backed object stores.
//
// One Store wraps a Hydra WorldState and hands out handles for each object
// type: key/value collections, SQL databases, and other typed surfaces.
// Every type shares the same underlying content-addressed block DAG, the
// same operation validation path, and the same watch infrastructure.
//
// This serves two audiences: standalone sync-engine consumers embedding a
// world in-process, and Spacewave SDK callers who want one-line access to
// typed storage without manual root-cursor wiring.
package worldstore

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	s4wave_kv_world "github.com/s4wave/spacewave/sdk/kv/world"
	s4wave_sql_world "github.com/s4wave/spacewave/sdk/sql/world"
	"github.com/sirupsen/logrus"
)

// Store provides typed access to world-backed object stores.
type Store struct {
	le *logrus.Entry
	ws world.WorldState
}

// Open constructs a Store over the given world state.
func Open(le *logrus.Entry, ws world.WorldState) (*Store, error) {
	if ws == nil {
		return nil, errNilWorldState
	}
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}
	return &Store{le: le, ws: ws}, nil
}

// KV opens or creates a key/value collection at name.
func (s *Store) KV(ctx context.Context, name string) (*s4wave_kv_world.WorldBackedStore, error) {
	return s4wave_kv_world.OpenOrCreate(ctx, s.le, s.ws, name)
}

// SQL opens or creates a SQL database at name.
func (s *Store) SQL(ctx context.Context, name string) (*s4wave_sql_world.WorldBackedSql, error) {
	obj, found, err := s.ws.GetObject(ctx, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errObjectNotFound(name)
	}
	var inner *s4wave_sql_world.WorldBackedSql
	if accessErr := obj.AccessWorldState(ctx, nil, func(root worldCursor) error {
		var openErr error
		inner, openErr = s4wave_sql_world.NewWorldBackedSql(ctx, root.Clone(), s.ws, name)
		return openErr
	}); accessErr != nil {
		return nil, accessErr
	}
	if inner == nil {
		return nil, errObjectNotFound(name)
	}
	return inner, nil
}

// WS exposes the underlying world state for direct access.
func (s *Store) WS() world.WorldState { return s.ws }

// Logger returns the store logger.
func (s *Store) Logger() *logrus.Entry { return s.le }

package kvtx_kvtest

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/pkg/errors"
)

// TestAll tests all tests for a kvtx store.
func TestAll(ctx context.Context, ktx kvtx.Store) error {
	tx, err := ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	keys := [][]byte{
		[]byte("ab"),
		[]byte("ba"),
		[]byte("ba1"),
		[]byte("ba2"),
		[]byte("bb"),
		[]byte("c"),
	}

	for _, k := range keys {
		ok, err := tx.Exists(k)
		if err != nil {
			return err
		}
		if ok {
			tx.Discard()
			return errors.Errorf("expected not exist: %s", string(k))
		}
	}
	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}

	for i := range keys {
		v := []byte(strconv.Itoa(i))
		if err := tx.Set(keys[i], v, time.Duration(0)); err != nil {
			tx.Discard()
			return err
		}
		val, ok, err := tx.Get(keys[i])
		if err != nil {
			return err
		}
		if !ok {
			tx.Discard()
			return errors.Errorf("expected key to exist: %s", string(keys[i]))
		}
		if bytes.Compare(val, v) != 0 {
			tx.Discard()
			return errors.Errorf("mismatch of value for key: %s", string(keys[i]))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	for i, k := range keys {
		v := []byte(strconv.Itoa(i))
		val, ok, err := tx.Get(k)
		if err != nil {
			return err
		}
		if !ok {
			tx.Discard()
			return errors.Errorf("expected key to exist: %s", string(k))
		}
		if bytes.Compare(val, v) != 0 {
			tx.Discard()
			return errors.Errorf("mismatch of value for key: %s", string(k))
		}
	}

	tx.Discard()

	tx, err = ktx.NewTransaction(true)
	if err != nil {
		return err
	}

	if err := tx.Delete(keys[0]); err != nil {
		tx.Discard()
		return err
	}

	_, ok, err := tx.Get(keys[0])
	if err == nil && ok {
		err = errors.Errorf("expected key to not exist after delete: %s", string(keys[0]))
	}
	if err != nil {
		tx.Discard()
		return err
	}

	tx.Discard()

	tx, err = ktx.NewTransaction(false)
	if err != nil {
		return err
	}

	val, ok, err := tx.Get(keys[0])
	if err == nil && !ok {
		err = errors.Errorf("expected key to exist after delete was discarded: %s", string(keys[0]))
	}
	if err == nil {
		if bytes.Compare(val, []byte("0")) != 0 {
			err = errors.Errorf("value mismatch for key: %s", string(keys[0]))
		}
	}
	if err != nil {
		tx.Discard()
		return err
	}

	tx.Discard()

	return nil
}

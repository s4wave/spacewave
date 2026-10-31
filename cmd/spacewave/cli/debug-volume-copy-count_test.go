//go:build !js

package spacewave_cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	block_store_sql "github.com/s4wave/spacewave/db/block/store/sql"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/sirupsen/logrus"
)

// TestVolumeDropSQL counts and copies a volume whose blocks a and b SQL holds
// for the store, and whose blocks b and d have upload markers. Both expect a
// dropped, b and d kept for their markers, c kept since only another store's
// rows hold it, and every key outside the blocks kept.
func TestVolumeDropSQL(t *testing.T) {
	// Open a fresh table in the database BLOCK_STORE_SQL_TEST_HOST names.
	host := os.Getenv("BLOCK_STORE_SQL_TEST_HOST")
	if host == "" {
		t.Skip("BLOCK_STORE_SQL_TEST_HOST names no MySQL-compatible server")
	}
	port, _ := strconv.ParseUint(os.Getenv("BLOCK_STORE_SQL_TEST_PORT"), 10, 32)
	conf := block_store_sql.Config{
		Host:     host,
		Port:     uint32(port),
		Database: os.Getenv("BLOCK_STORE_SQL_TEST_DATABASE"),
		User:     os.Getenv("BLOCK_STORE_SQL_TEST_USER"),
		Password: os.Getenv("BLOCK_STORE_SQL_TEST_PASSWORD"),
		Table:    "test_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_blocks",
		MaxConns: 1,
	}
	ctx := t.Context()
	db, err := block_store_sql.Open(&conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := block_store_sql.EnsureTable(ctx, db, conf.Table); err != nil {
		t.Fatal(err)
	}
	rows := block_store_sql.NewStore(logrus.NewEntry(logrus.New()), db, conf.Table, "world")
	defer rows.Close()
	other := block_store_sql.NewStore(logrus.NewEntry(logrus.New()), db, conf.Table, "other")

	// Write the blocks to the volume, a and b also to the store's rows and c
	// and d to another store's, with markers for b and d.
	src := filepath.Join(t.TempDir(), "src.s4db")
	sdb, err := s4db.Open(src, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	keys := store_kvkey.NewDefaultKVKey()
	tx, err := sdb.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	blockKeys := make(map[string][]byte)
	for _, name := range []string{"a", "b", "c", "d"} {
		data := []byte("block " + name)
		store := rows
		if name == "c" || name == "d" {
			store = other
		}
		ref, _, err := store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		rm, err := ref.MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		blockKeys[name] = keys.GetBlockKey(rm)
		if err := tx.Set(ctx, blockKeys[name], data); err != nil {
			t.Fatal(err)
		}
		if name == "b" || name == "d" {
			marker := append(keys.GetObjectStorePrefixByID("p/prov/acct/blk/world/writeback"), "marked/"+ref.GetHash().MarshalString()...)
			if err := tx.Set(ctx, marker, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	state := append(keys.GetObjectStorePrefixByID("p/prov/acct/so"), "world/state"...)
	if err := tx.Set(ctx, state, []byte("state")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), store_kvkey.MarshalFormatVersion(store_kvkey.FormatVersion)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sdb.Close(); err != nil {
		t.Fatal(err)
	}

	// The count drops a alone and names each block's family.
	var out, log bytes.Buffer
	if err := countVolumeDrop(ctx, src, &sqlDrop{conf: conf, storeID: "world"}, &out, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "keeping the blocks of 2 upload markers") {
		t.Fatalf("log %q names no 2 markers", log.String())
	}
	for _, want := range []string{
		`(?m)^drop: 1 keys,`,
		`(?m)^keep: 7 keys,`,
		`(?m)^ +1 keys +\S+ \S+  ` + dropCountDropped + `$`,
		`(?m)^ +2 keys +\S+ \S+  ` + dropCountMarked + `$`,
		`(?m)^ +1 keys +\S+ \S+  ` + dropCountLacks + `$`,
		`(?m)^ +1 keys +\S+ \S+  h/objs/p/prov/acct/so/world/$`,
	} {
		if !regexp.MustCompile(want).MatchString(out.String()) {
			t.Fatalf("count output lacks %s:\n%s", want, out.String())
		}
	}

	// The copy drops a alone and keeps the other keys.
	dst := filepath.Join(t.TempDir(), "dst.s4db")
	if err := copyVolumeFile(ctx, src, dst, nil, &sqlDrop{conf: conf, storeID: "world"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	ddb, err := s4db.Open(dst, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer ddb.Close()
	rtx, err := ddb.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for name, key := range blockKeys {
		_, found, err := rtx.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if found != (name != "a") {
			t.Fatalf("block %s found %v", name, found)
		}
	}
	if value, found, err := rtx.Get(ctx, state); err != nil || !found || string(value) != "state" {
		t.Fatalf("state key: found %v value %q err %v", found, value, err)
	}
}

//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"context"
	"net"
	"path/filepath"
	"strings"

	mysql_devserver "github.com/s4wave/spacewave/db/sql/mysql/devserver"
)

// sqlSchemaDatabase is the database name the Vite processes see in
// DATABASE_URL.
const sqlSchemaDatabase = "app"

// readSqlSchema reads the SQL schema paths, relative to sourcePath. Returns the
// files read and the schema as one text, the files joined in order. The joined
// schema keys the Vite processes so that a schema edit starts a fresh database.
func readSqlSchema(sourcePath string, paths []string) ([]string, string, error) {
	// Resolve each path against the project root.
	absPaths := make([]string, len(paths))
	for i, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(sourcePath, path)
		}
		absPaths[i] = path
	}

	// Read the files and join their texts, ending each file's last statement.
	files, texts, err := mysql_devserver.ReadSchemaFiles(absPaths)
	if err != nil {
		return nil, "", err
	}
	return files, strings.Join(texts, "\n;\n"), nil
}

// startSqlSchemaServer serves an in-memory database built from the schema on
// a loopback port until the returned server closes.
func startSqlSchemaServer(ctx context.Context, schema string) (*mysql_devserver.Server, error) {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return mysql_devserver.Start(ctx, lis, sqlSchemaDatabase, []string{schema})
}

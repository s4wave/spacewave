package mysql_devserver

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// SchemaFileSuffix ends the name of each schema file a directory contributes.
const SchemaFileSuffix = ".sql"

// ReadSchemaFiles reads the SQL schema at each path in order. A directory
// contributes its .sql files sorted by name, the order migration files are
// numbered in. Subdirectories are not read. Returns the files read and their
// texts, in the same order.
func ReadSchemaFiles(paths []string) (files []string, texts []string, err error) {
	for _, path := range paths {
		// Expand a directory to its .sql files in name order.
		pathFiles := []string{path}
		st, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		if st.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, nil, err
			}
			pathFiles = pathFiles[:0]
			for _, ent := range entries {
				if !ent.IsDir() && strings.HasSuffix(ent.Name(), SchemaFileSuffix) {
					pathFiles = append(pathFiles, filepath.Join(path, ent.Name()))
				}
			}
			slices.Sort(pathFiles)
		}

		// Read each file as one schema text.
		for _, file := range pathFiles {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, nil, errors.Wrap(err, "read schema file")
			}
			files = append(files, file)
			texts = append(texts, string(data))
		}
	}
	return files, texts, nil
}

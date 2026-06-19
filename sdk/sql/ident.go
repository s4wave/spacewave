package s4wave_sql

import (
	"strings"

	"github.com/pkg/errors"
)

// QuoteIdentifier returns a MySQL-compatible quoted identifier.
func QuoteIdentifier(identifier string) (string, error) {
	if identifier == "" {
		return "", errors.New("sql: identifier is required")
	}
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`", nil
}

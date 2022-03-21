package sql_gorm

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

var (
	// CreateClauses create clauses
	CreateClauses = []string{"INSERT", "VALUES", "ON CONFLICT"}
	// QueryClauses query clauses
	QueryClauses = []string{}
	// UpdateClauses update clauses
	UpdateClauses = []string{"UPDATE", "SET", "WHERE", "ORDER BY", "LIMIT"}
	// DeleteClauses delete clauses
	DeleteClauses = []string{"DELETE", "FROM", "WHERE", "ORDER BY", "LIMIT"}
)

// Note: adapted from https://github.com/go-gorm/mysql
// currently synced w/ version efbd06126e4bf540aebbf8f11cdd2c5486dfa227

// Dialector implements the Dialector interface from gorm.
type Dialector struct {
	db *sql.DB
}

// NewDialector constructs a new dialector from a sql store.
func NewDialector(db *sql.DB) gorm.Dialector {
	return &Dialector{db: db}
}

// Name returns the name of the dialector.
func (d *Dialector) Name() string {
	return "hydra"
}

// Initialize initializes the dialector with a db.
func (d *Dialector) Initialize(db *gorm.DB) (err error) {
	// register callbacks
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
		CreateClauses: CreateClauses,
		QueryClauses:  []string{},
		UpdateClauses: UpdateClauses,
		DeleteClauses: DeleteClauses,
	})
	for k, v := range d.ClauseBuilders() {
		db.ClauseBuilders[k] = v
	}
	return nil
}

// ClauseBuilders returns the set of clause builders.
func (d *Dialector) ClauseBuilders() map[string]clause.ClauseBuilder {
	return map[string]clause.ClauseBuilder{
		"ON CONFLICT": func(c clause.Clause, builder clause.Builder) {
			if onConflict, ok := c.Expression.(clause.OnConflict); ok {
				_, _ = builder.WriteString("ON DUPLICATE KEY UPDATE ")
				if len(onConflict.DoUpdates) == 0 {
					if s := builder.(*gorm.Statement).Schema; s != nil {
						var column clause.Column
						onConflict.DoNothing = false

						if s.PrioritizedPrimaryField != nil {
							column = clause.Column{Name: s.PrioritizedPrimaryField.DBName}
						} else if len(s.DBNames) > 0 {
							column = clause.Column{Name: s.DBNames[0]}
						}

						if column.Name != "" {
							onConflict.DoUpdates = []clause.Assignment{{Column: column, Value: column}}
						}
					}
				}

				for idx, assignment := range onConflict.DoUpdates {
					if idx > 0 {
						_ = builder.WriteByte(',')
					}

					builder.WriteQuoted(assignment.Column)
					_ = builder.WriteByte('=')
					if column, ok := assignment.Value.(clause.Column); ok && column.Table == "excluded" {
						column.Table = ""
						_, _ = builder.WriteString("VALUES(")
						builder.WriteQuoted(column)
						_ = builder.WriteByte(')')
					} else {
						builder.AddVar(builder, assignment.Value)
					}
				}
			} else {
				c.Build(builder)
			}
		},
		"VALUES": func(c clause.Clause, builder clause.Builder) {
			if values, ok := c.Expression.(clause.Values); ok && len(values.Columns) == 0 {
				_, _ = builder.WriteString("VALUES()")
				return
			}
			c.Build(builder)
		},
	}
}

func (d *Dialector) DefaultValueOf(field *schema.Field) clause.Expression {
	return clause.Expr{SQL: "DEFAULT"}
}

func (d *Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return Migrator{migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   d,
		CreateIndexAfterCreateTable: true,
	}}, d}
}

func (d *Dialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v interface{}) {
	_ = writer.WriteByte('?')
}

func (d *Dialector) QuoteTo(writer clause.Writer, str string) {
	_ = writer.WriteByte('`')
	if strings.Contains(str, ".") {
		for idx, str := range strings.Split(str, ".") {
			if idx > 0 {
				_, _ = writer.WriteString(".`")
			}
			_, _ = writer.WriteString(str)
			_ = writer.WriteByte('`')
		}
	} else {
		_, _ = writer.WriteString(str)
		_ = writer.WriteByte('`')
	}
}

func (d *Dialector) Explain(sql string, vars ...interface{}) string {
	return logger.ExplainSQL(sql, nil, `'`, vars...)
}

func (d *Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "boolean"
	case schema.Int, schema.Uint:
		sqlType := "bigint"
		switch {
		case field.Size <= 8:
			sqlType = "tinyint"
		case field.Size <= 16:
			sqlType = "smallint"
		case field.Size <= 24:
			sqlType = "mediumint"
		case field.Size <= 32:
			sqlType = "int"
		}

		if field.DataType == schema.Uint {
			sqlType += " unsigned"
		}

		if field.AutoIncrement {
			sqlType += " AUTO_INCREMENT"
		}
		return sqlType
	case schema.Float:
		if field.Precision > 0 {
			return fmt.Sprintf("decimal(%d, %d)", field.Precision, field.Scale)
		}

		if field.Size <= 32 {
			return "float"
		}
		return "double"
	case schema.String:
		size := field.Size

		if size == 0 {
			hasIndex := field.TagSettings["INDEX"] != "" || field.TagSettings["UNIQUE"] != ""
			// TEXT, GEOMETRY or JSON column can't have a default value
			if field.PrimaryKey || field.HasDefaultValue || hasIndex {
				size = 191 // utf8mb4
			}
		}

		if size >= 65536 && size <= int(math.Pow(2, 24)) {
			return "mediumtext"
		} else if size > int(math.Pow(2, 24)) || size <= 0 {
			return "longtext"
		}
		return fmt.Sprintf("varchar(%d)", size)
	case schema.Time:
		precision := ""

		if field.Precision > 0 {
			precision = fmt.Sprintf("(%d)", field.Precision)
		}

		if field.NotNull || field.PrimaryKey {
			return "datetime" + precision
		}
		return "datetime" + precision + " NULL"
	case schema.Bytes:
		if field.Size > 0 && field.Size < 65536 {
			return fmt.Sprintf("varbinary(%d)", field.Size)
		}

		if field.Size >= 65536 && field.Size <= int(math.Pow(2, 24)) {
			return "mediumblob"
		}

		return "longblob"
	}

	return string(field.DataType)
}

func (d *Dialector) SavePoint(tx *gorm.DB, name string) error {
	// tx.Exec("SAVEPOINT " + name)
	// return nil
	return gorm.ErrNotImplemented
}

func (d *Dialector) RollbackTo(tx *gorm.DB, name string) error {
	// tx.Exec("ROLLBACK TO SAVEPOINT " + name)
	// return nil
	return gorm.ErrNotImplemented
}

// _ is a type assertion
var _ gorm.Dialector = ((*Dialector)(nil))

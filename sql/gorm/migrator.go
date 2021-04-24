package sql_gorm

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// Migrator migrates the schema changes between code versions.
// see information_schema.go:
//  - numeric_precision: not supported
//  - date_time_precision: not supported
type Migrator struct {
	migrator.Migrator
	*Dialector
}

type Column struct {
	name     string
	nullable sql.NullString
	datatype string
}

func (c Column) Name() string {
	return c.name
}

func (c Column) DatabaseTypeName() string {
	return c.datatype
}

func (c Column) Length() (length int64, ok bool) {
	length = 0
	return
}

func (c Column) Nullable() (nullable bool, ok bool) {
	if c.nullable.Valid {
		nullable, ok = c.nullable.String == "YES", true
	} else {
		nullable, ok = false, false
	}
	return
}

func (c Column) DecimalSize() (precision int64, scale int64, ok bool) {
	// unknown
	precision, scale, ok = 0, 0, false
	return
}

func (m Migrator) HasTable(value interface{}) bool {
	var count int64

	m.RunWithValue(value, func(stmt *gorm.Statement) error {
		currentDatabase := m.DB.Migrator().CurrentDatabase()
		return m.DB.Raw(
			"SELECT count(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ? AND table_type = ?",
			currentDatabase,
			stmt.Table,
			"BASE TABLE",
		).Row().Scan(&count)
	})

	return count > 0
}

func (m Migrator) FullDataTypeOf(field *schema.Field) clause.Expr {
	expr := m.Migrator.FullDataTypeOf(field)

	if value, ok := field.TagSettings["COMMENT"]; ok {
		expr.SQL += " COMMENT " + m.Dialector.Explain("?", value)
	}

	return expr
}

func (m Migrator) AlterColumn(value interface{}, field string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if field := stmt.Schema.LookUpField(field); field != nil {
			return m.DB.Exec(
				"ALTER TABLE ? MODIFY COLUMN ? ?",
				clause.Table{Name: stmt.Table}, clause.Column{Name: field.DBName}, m.FullDataTypeOf(field),
			).Error
		}
		return fmt.Errorf("failed to look up field with name: %s", field)
	})
}

func (m Migrator) RenameIndex(value interface{}, oldName, newName string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		return m.DB.Exec(
			"ALTER TABLE ? RENAME INDEX ? TO ?",
			clause.Table{Name: stmt.Table}, clause.Column{Name: oldName}, clause.Column{Name: newName},
		).Error
	})
}

func (m Migrator) DropTable(values ...interface{}) error {
	values = m.ReorderModels(values, false)
	tx := m.DB.Session(&gorm.Session{})
	tx.Exec("SET FOREIGN_KEY_CHECKS = 0;")
	for i := len(values) - 1; i >= 0; i-- {
		if err := m.RunWithValue(values[i], func(stmt *gorm.Statement) error {
			return tx.Exec("DROP TABLE IF EXISTS ? CASCADE", clause.Table{Name: stmt.Table}).Error
		}); err != nil {
			return err
		}
	}
	tx.Exec("SET FOREIGN_KEY_CHECKS = 1;")
	return nil
}

func (m Migrator) DropConstraint(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, chk, table := m.GuessConstraintAndTable(stmt, name)
		if chk != nil {
			return m.DB.Exec("ALTER TABLE ? DROP CHECK ?", clause.Table{Name: stmt.Table}, clause.Column{Name: chk.Name}).Error
		}
		if constraint != nil {
			name = constraint.Name
		}

		return m.DB.Exec(
			"ALTER TABLE ? DROP FOREIGN KEY ?", clause.Table{Name: table}, clause.Column{Name: name},
		).Error
	})
}

func (m Migrator) ColumnTypes(value interface{}) (columnTypes []gorm.ColumnType, err error) {
	columnTypes = make([]gorm.ColumnType, 0)
	err = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		var (
			currentDatabase = m.DB.Migrator().CurrentDatabase()
			columnTypeSQL   = "SELECT column_name, is_nullable, data_type FROM information_schema.columns WHERE table_schema = ? AND table_name = ?"
		)

		columns, err := m.DB.Raw(columnTypeSQL, currentDatabase, stmt.Table).Rows()
		if err != nil {
			return err
		}
		defer columns.Close()

		for columns.Next() {
			var column Column
			var values = []interface{}{&column.name, &column.nullable, &column.datatype}

			if err = columns.Scan(values...); err != nil {
				return err
			}
			columnTypes = append(columnTypes, column)
		}

		return err
	})
	return
}

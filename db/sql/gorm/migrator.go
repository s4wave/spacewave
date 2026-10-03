package sql_gorm

import (
	"database/sql"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// Migrator migrates the schema changes between code versions.
// see information_schema.go:
//   - numeric_precision: not supported
//   - date_time_precision: not supported
type Migrator struct {
	migrator.Migrator
	*Dialector
}

func (m Migrator) FullDataTypeOf(field *schema.Field) clause.Expr {
	expr := m.Migrator.FullDataTypeOf(field)

	if value, ok := field.TagSettings["COMMENT"]; ok {
		expr.SQL += " COMMENT " + m.Explain("?", value)
	}

	return expr
}

func (m Migrator) AlterColumn(value any, field string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if field := stmt.Schema.LookUpField(field); field != nil {
			return m.DB.Exec(
				"ALTER TABLE ? MODIFY COLUMN ? ?",
				clause.Table{Name: stmt.Table}, clause.Column{Name: field.DBName}, m.FullDataTypeOf(field),
			).Error
		}
		return errors.Errorf("failed to look up field with name: %s", field)
	})
}

func (m Migrator) RenameColumn(value any, oldName, newName string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		/*
			if !m.Dialector.DontSupportRenameColumn {
				return m.Migrator.RenameColumn(value, oldName, newName)
			}
		*/

		// Resolve the old column name and schema for the rename.
		var field *schema.Field
		if f := stmt.Schema.LookUpField(oldName); f != nil {
			oldName = f.DBName
			field = f
		}

		// Resolve the new column name and its schema when available.
		if f := stmt.Schema.LookUpField(newName); f != nil {
			newName = f.DBName
			field = f
		}

		// Rename the column using the resolved field definition.
		if field != nil {
			return m.DB.Exec(
				"ALTER TABLE ? CHANGE ? ? ?",
				clause.Table{Name: stmt.Table}, clause.Column{Name: oldName},
				clause.Column{Name: newName}, m.FullDataTypeOf(field),
			).Error
		}

		return errors.Errorf("failed to look up field with name: %s", newName)
	})
}

func (m Migrator) RenameIndex(value any, oldName, newName string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		return m.DB.Exec(
			"ALTER TABLE ? RENAME INDEX ? TO ?",
			clause.Table{Name: stmt.Table}, clause.Column{Name: oldName}, clause.Column{Name: newName},
		).Error
	})
}

// DropTable drops the tables of the given models in dependency order.
func (m Migrator) DropTable(values ...any) error {
	values = m.ReorderModels(values, false)
	return m.DB.Connection(func(tx *gorm.DB) (err error) {
		// Suspend foreign key checks on this connection for table removal.
		if err := tx.Exec("SET FOREIGN_KEY_CHECKS = 0;").Error; err != nil {
			return err
		}

		// Restore foreign key checks even when a drop fails.
		defer func() {
			if rerr := tx.Exec("SET FOREIGN_KEY_CHECKS = 1;").Error; err == nil {
				err = rerr
			}
		}()

		// Drop dependent tables before the tables they reference.
		for _, v := range slices.Backward(values) {
			if err := m.RunWithValue(v, func(stmt *gorm.Statement) error {
				return tx.Exec("DROP TABLE IF EXISTS ? CASCADE", clause.Table{Name: stmt.Table}).Error
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m Migrator) DropConstraint(value any, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		// Resolve the constraint and distinguish a check from a foreign key.
		constraint, table := m.GuessConstraintInterfaceAndTable(stmt, name)
		chk, chkOk := constraint.(*schema.CheckConstraint)
		if chkOk {
			constraint = nil
		}

		// Remove a check constraint with the table check syntax.
		if chk != nil {
			return m.DB.Exec("ALTER TABLE ? DROP CHECK ?", clause.Table{Name: stmt.Table}, clause.Column{Name: chk.Name}).Error
		}

		// Resolve the foreign key name before removing it.
		if constraint != nil {
			name = constraint.GetName()
		}

		return m.DB.Exec(
			"ALTER TABLE ? DROP FOREIGN KEY ?", clause.Table{Name: table}, clause.Column{Name: name},
		).Error
	})
}

func (m Migrator) ColumnTypes(value any) (columnTypes []gorm.ColumnType, err error) {
	// Collect column metadata from the driver and information schema.
	columnTypes = make([]gorm.ColumnType, 0)
	err = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		// Prepare the database and metadata query for the requested table.
		var (
			currentDatabase = m.DB.Migrator().CurrentDatabase()
			// columnTypeSQL   = "SELECT column_name, is_nullable, data_type FROM information_schema.columns WHERE table_schema = ? AND table_name = ?"
			columnTypeSQL = "SELECT column_name, column_default, is_nullable = 'YES', data_type, character_maximum_length, column_type, column_key, extra, column_comment"
		)

		// Open a table query to obtain the driver column descriptions.
		rows, err := m.DB.Session(&gorm.Session{}).Table(stmt.Table).Limit(1).Rows()
		if err != nil {
			return err
		}

		// Read the driver descriptions and release the table rows.
		rawColumnTypes, err := rows.ColumnTypes()
		if err := rows.Close(); err != nil {
			return err
		}

		// Restrict the metadata query to this table in column order.
		columnTypeSQL += " FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ORDINAL_POSITION"

		// Open the information schema rows for the table columns.
		columns, rowErr := m.DB.Raw(columnTypeSQL, currentDatabase, stmt.Table).Rows()
		if rowErr != nil {
			return rowErr
		}
		defer columns.Close()

		// Combine information schema values with each driver description.
		for columns.Next() {
			// Prepare destinations for the column metadata row.
			var (
				column     migrator.ColumnType
				extraValue sql.NullString
				columnKey  sql.NullString
				values     = []any{
					&column.NameValue,
					&column.DefaultValueValue,
					&column.NullableValue,
					&column.DataTypeValue,
					&column.LengthValue,
					&column.ColumnTypeValue,
					&columnKey,
					&extraValue,
					&column.CommentValue,
				}
			)

			// Read the information schema values for this column.
			if scanErr := columns.Scan(values...); scanErr != nil {
				return scanErr
			}

			// Interpret the column key as primary or unique membership.
			column.PrimaryKeyValue = sql.NullBool{Bool: false, Valid: true}
			column.UniqueValue = sql.NullBool{Bool: false, Valid: true}
			switch columnKey.String {
			case "PRI":
				column.PrimaryKeyValue = sql.NullBool{Bool: true, Valid: true}
			case "UNI":
				column.UniqueValue = sql.NullBool{Bool: true, Valid: true}
			}

			// Identify columns whose values are assigned by auto increment.
			if strings.Contains(extraValue.String, "auto_increment") {
				column.AutoIncrementValue = sql.NullBool{Bool: true, Valid: true}
			}

			// Normalize the column default by removing surrounding quotes.
			column.DefaultValueValue.String = strings.Trim(column.DefaultValueValue.String, "'")

			// Attach the matching driver description to the column metadata.
			for _, c := range rawColumnTypes {
				if c.Name() == column.NameValue.String {
					column.SQLColumnType = c
					break
				}
			}

			// Include the completed column in the table metadata.
			columnTypes = append(columnTypes, column)
		}

		return err
	})
	return
}

func (m Migrator) GetTables() (tableList []string, err error) {
	err = m.DB.Raw("SELECT TABLE_NAME FROM information_schema.tables where TABLE_SCHEMA=?", m.CurrentDatabase()).
		Scan(&tableList).Error
	return
}

// _ is a type assertion
var _ gorm.Migrator = (*Migrator)(nil)

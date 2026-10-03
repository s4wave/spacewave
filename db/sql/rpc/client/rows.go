package sql_rpc_client

import (
	"database/sql/driver"
	"errors"
	"io"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
)

type rows struct {
	client    sql_rpc.SRPCSqlOps_QueryClient
	columns   []*hydra_sql.ColumnSchema
	buffer    []*hydra_sql.Row
	closed    bool
	closeSent bool
}

func newRows(client sql_rpc.SRPCSqlOps_QueryClient, columns []*hydra_sql.ColumnSchema) *rows {
	return &rows{
		client:  client,
		columns: columns,
	}
}

// Columns returns the result column names.
func (r *rows) Columns() []string {
	columns := make([]string, len(r.columns))
	for i, column := range r.columns {
		columns[i] = column.GetName()
	}
	return columns
}

// ColumnTypeDatabaseTypeName returns the database-specific column type.
func (r *rows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.columns) {
		return ""
	}
	return r.columns[index].GetDatabaseTypeName()
}

// Close closes the rows iterator.
func (r *rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if !r.closeSent {
		r.closeSent = true
		err := r.client.Send(&sql_rpc.SqlQueryRequest{
			Body: &sql_rpc.SqlQueryRequest_Close{Close: true},
		})
		if err == nil {
			_, _ = r.client.Recv()
		}
		if cerr := r.client.Close(); err == nil {
			err = cerr
		}
		return mapTxError(err)
	}
	return mapTxError(r.client.Close())
}

// Next populates dest with the values of the next row.
func (r *rows) Next(dest []driver.Value) error {
	// Stop row iteration when the SQL query is closed.
	if r.closed {
		return io.EOF
	}

	// Request SQL result batches until a row is buffered.
	for len(r.buffer) == 0 {
		// Request the next row from the remote SQL query.
		if err := r.client.Send(&sql_rpc.SqlQueryRequest{
			Body: &sql_rpc.SqlQueryRequest_Next{Next: 1},
		}); err != nil {
			r.closed = true
			return mapTxError(err)
		}

		// Receive the SQL query response and handle stream failures.
		resp, err := r.client.Recv()
		if err != nil {
			r.closed = true
			_ = r.client.Close()
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			return mapTxError(err)
		}

		// Handle SQL query errors and remote query closure.
		if errStr := resp.GetReqError(); errStr != "" {
			r.closed = true
			_ = r.client.Close()
			return errors.New(errStr)
		}
		if resp.GetClosed() {
			r.closed = true
			_ = r.client.Close()
			return io.EOF
		}

		// Buffer the SQL rows returned by the query batch.
		batch := resp.GetBatch()
		if batch == nil {
			r.closed = true
			return errors.New("sql rpc: expected query batch or closed")
		}
		r.buffer = append(r.buffer, batch.GetRows()...)
	}

	// Consume the first buffered SQL row and release its buffer slot.
	row := r.buffer[0]
	copy(r.buffer, r.buffer[1:])
	r.buffer[len(r.buffer)-1] = nil
	r.buffer = r.buffer[:len(r.buffer)-1]

	// Convert the SQL row values into the driver destination.
	values := row.GetValues()
	for i := range dest {
		if i < len(values) {
			dest[i] = sql_rpc.SqlValueToDriverValue(values[i])
		} else {
			dest[i] = nil
		}
	}
	return nil
}

// _ is a type assertion.
var (
	_ driver.Rows                           = (*rows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*rows)(nil)
)

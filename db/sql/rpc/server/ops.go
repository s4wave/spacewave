package sql_rpc_server

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"

	hydra_sql "github.com/s4wave/spacewave/db/sql"
	sql_rpc "github.com/s4wave/spacewave/db/sql/rpc"
)

// Ops implements SQL transaction operations.
type Ops struct {
	// ops is the underlying SQL ops interface.
	ops hydra_sql.SqlOps
}

// NewOps constructs a new SqlOps service.
func NewOps(ops hydra_sql.SqlOps) *Ops {
	return &Ops{ops: ops}
}

// Exec executes a SQL statement.
func (o *Ops) Exec(ctx context.Context, req *sql_rpc.SqlExecRequest) (*sql_rpc.SqlExecResponse, error) {
	args := sql_rpc.SqlValuesToNamedValues(req.GetArgs())
	result, err := o.ops.ExecContext(ctx, req.GetQuery(), args)
	if errors.Is(err, driver.ErrSkip) {
		result, err = o.ops.Exec(req.GetQuery(), sql_rpc.NamedValuesToValues(args))
	}

	resp := &sql_rpc.SqlExecResponse{}
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}
	if result == nil {
		return resp, nil
	}
	if lastInsertID, err := result.LastInsertId(); err == nil {
		resp.LastInsertId = lastInsertID
	}
	if rowsAffected, err := result.RowsAffected(); err == nil {
		resp.RowsAffected = rowsAffected
	}
	return resp, nil
}

// Query executes a SQL query with explicit row iteration control.
func (o *Ops) Query(strm sql_rpc.SRPCSqlOps_QueryStream) error {
	initReq, err := strm.Recv()
	if err != nil {
		return err
	}
	init := initReq.GetInit()
	if init == nil {
		return sendQueryReqError(strm, "expected init request")
	}

	args := sql_rpc.SqlValuesToNamedValues(init.GetArgs())
	rows, err := o.ops.QueryContext(strm.Context(), init.GetQuery(), args)
	if errors.Is(err, driver.ErrSkip) {
		rows, err = o.ops.Query(init.GetQuery(), sql_rpc.NamedValuesToValues(args))
	}
	if err != nil {
		return sendQueryReqError(strm, err.Error())
	}
	if rows == nil {
		return sendQueryReqError(strm, "query returned nil rows")
	}
	defer rows.Close()

	columns := rows.Columns()
	columnSchemas := make([]*hydra_sql.ColumnSchema, len(columns))
	columnTypes, _ := rows.(driver.RowsColumnTypeDatabaseTypeName)
	for i, name := range columns {
		columnSchemas[i] = &hydra_sql.ColumnSchema{Name: name}
		if columnTypes != nil {
			columnSchemas[i].DatabaseTypeName = columnTypes.ColumnTypeDatabaseTypeName(i)
		}
	}

	if err := strm.Send(&sql_rpc.SqlQueryResponse{
		Body: &sql_rpc.SqlQueryResponse_Ack{
			Ack: &sql_rpc.SqlQueryAck{Columns: columnSchemas},
		},
	}); err != nil {
		return err
	}

	dest := make([]driver.Value, len(columns))
	done := false
	for {
		req, err := strm.Recv()
		if err != nil {
			return err
		}
		switch m := req.GetBody().(type) {
		case *sql_rpc.SqlQueryRequest_Init:
			return sendQueryReqError(strm, "init sent multiple times")
		case *sql_rpc.SqlQueryRequest_Close:
			if m.Close {
				return strm.Send(&sql_rpc.SqlQueryResponse{
					Body: &sql_rpc.SqlQueryResponse_Closed{Closed: true},
				})
			}
		case *sql_rpc.SqlQueryRequest_Next:
			if done {
				return strm.Send(&sql_rpc.SqlQueryResponse{
					Body: &sql_rpc.SqlQueryResponse_Closed{Closed: true},
				})
			}
			count := m.Next
			if count == 0 {
				count = 1
			}
			batch := &hydra_sql.RowBatch{}
			for i := uint32(0); i < count; i++ {
				clear(dest)
				err := rows.Next(dest)
				if err == io.EOF {
					done = true
					break
				}
				if err != nil {
					return sendQueryReqError(strm, err.Error())
				}
				row := &hydra_sql.Row{Values: make([]*hydra_sql.SqlValue, len(dest))}
				for i, value := range dest {
					wireValue, err := sql_rpc.DriverValueToSqlValue(value)
					if err != nil {
						return sendQueryReqError(strm, err.Error())
					}
					row.Values[i] = wireValue
				}
				batch.Rows = append(batch.Rows, row)
			}
			if len(batch.Rows) == 0 && done {
				return strm.Send(&sql_rpc.SqlQueryResponse{
					Body: &sql_rpc.SqlQueryResponse_Closed{Closed: true},
				})
			}
			if err := strm.Send(&sql_rpc.SqlQueryResponse{
				Body: &sql_rpc.SqlQueryResponse_Batch{Batch: batch},
			}); err != nil {
				return err
			}
		}
	}
}

func sendQueryReqError(strm sql_rpc.SRPCSqlOps_QueryStream, errStr string) error {
	return strm.Send(&sql_rpc.SqlQueryResponse{
		Body: &sql_rpc.SqlQueryResponse_ReqError{ReqError: errStr},
	})
}

// _ is a type assertion.
var _ sql_rpc.SRPCSqlOpsServer = ((*Ops)(nil))

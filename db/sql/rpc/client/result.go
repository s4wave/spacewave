package sql_rpc_client

// result implements database/sql/driver.Result.
type result struct {
	lastInsertID int64
	rowsAffected int64
}

// LastInsertId returns the last insert id returned by the remote driver.
func (r *result) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

// RowsAffected returns the affected row count returned by the remote driver.
func (r *result) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

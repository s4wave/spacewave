package s4wave_sql_world

// SqlDbTypeID is the world ObjectType id for SQL databases.
//
// The id is the stable identity of the SQL database object across every build,
// so it lives in this unconstrained file: the SqlSetRootOp world operation (and
// the kvtx/sql_lite and TinyGo browser builds that compile it) resolves the id
// without the SRPC-server-backed factory in objecttype.go, which is gated to
// !tinygo && !sql_lite.
const SqlDbTypeID = "sql/db"

package s4wave_sql_world

// SqlDbTypeID is the world ObjectType id for SQL databases.
//
// The id is the stable identity of the SQL database object across every build,
// so it lives in this unconstrained file: the SqlSetRootOp world operation and
// TinyGo browser builds resolve the id without the SRPC-server-backed factory
// in objecttype.go, which is gated to !tinygo.
const SqlDbTypeID = "sql/db"

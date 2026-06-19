package s4wave_sql

import "github.com/aperturerobotics/cayley/quad"

// PredSqlQueryAgainst links a SQL query or result to the database it targets.
var PredSqlQueryAgainst = quad.IRI("against")

// PredSqlQueryProducedBy links a SQL query result to the query that produced it.
var PredSqlQueryProducedBy = quad.IRI("produced-by")

// PredSqlSchemaInDb links a SQL schema to the database that contains it.
var PredSqlSchemaInDb = quad.IRI("schema-in-db")

// PredSqlTableViewAgainstSchema links a SQL table view to the schema it reads.
var PredSqlTableViewAgainstSchema = quad.IRI("table-view-against-schema")

// PredSqlWorkbenchAgainstDb links a SQL workbench to the database it explores.
var PredSqlWorkbenchAgainstDb = quad.IRI("workbench-against-db")

// PredSqlWorkbenchPinnedQuery links a SQL workbench to a pinned query.
var PredSqlWorkbenchPinnedQuery = quad.IRI("workbench-pinned-query")

// PredSqlWorkbenchOpenTab links a SQL workbench to an object opened in a tab.
var PredSqlWorkbenchOpenTab = quad.IRI("workbench-open-tab")

// CREATE_DOCS_OP_ID is the operation type ID for the docs create op that builds
// its content under the World writer. Only stored ops carry it: they replay
// through the current handler of an unpinned op id, so its handler stays and
// must keep their result. New creates send CREATE_DOCS_STAGED_OP_ID.
export const CREATE_DOCS_OP_ID = 'notes/docs/create'

// CREATE_DOCS_STAGED_OP_ID is the operation type ID for the docs create op
// whose sender builds the content in a World stage; see
// CreateDocumentationStagedOp.
export const CREATE_DOCS_STAGED_OP_ID = 'notes/docs/create-staged'

// DOCS_OBJECT_KEY is the default object key for the docs quickstart.
export const DOCS_OBJECT_KEY = 'docs/documentation'

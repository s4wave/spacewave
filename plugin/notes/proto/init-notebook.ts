// INIT_NOTEBOOK_OP_ID is the operation type ID for the notebook init op that
// builds its content under the World writer. Only stored ops carry it: they
// replay through the current handler of an unpinned op id, so its handler stays
// and must keep their result. New creates send INIT_NOTEBOOK_STAGED_OP_ID.
export const INIT_NOTEBOOK_OP_ID = 'notes/notebook/init'

// INIT_NOTEBOOK_STAGED_OP_ID is the operation type ID for the notebook init op
// whose sender builds the content in a World stage; see InitNotebookStagedOp.
export const INIT_NOTEBOOK_STAGED_OP_ID = 'notes/notebook/init-staged'

// NOTEBOOK_OBJECT_KEY is the default object key for the notebook quickstart.
export const NOTEBOOK_OBJECT_KEY = 'notebook'
